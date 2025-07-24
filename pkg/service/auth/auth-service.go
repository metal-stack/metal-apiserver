package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/service/token"
	"github.com/metal-stack/metal-lib/auditing"
)

const (
	providerKey = "provider"
)

type Config struct {
	TokenService token.TokenService
	Repo         *repository.Store
	Auditing     auditing.Auditing
	Log          *slog.Logger
	CallbackUrl  string // will replace `"{" + providerKey + ""}"` with the actual provider name
	FrontEndUrl  *url.URL
	CookieMaxAge time.Duration
}

type providerUser struct {
	login     string
	name      string
	email     string
	avatarUrl string
	provider  string
}

type providerBackend interface {
	Name() string
	User(ctx context.Context, user goth.User) (*providerUser, error)
	EndSessionRedirectURL() string
}

type auth struct {
	providerBackends map[string]providerBackend
	tokenService     token.TokenService
	audit            auditing.Auditing
	log              *slog.Logger
	frontEndUrl      *url.URL
	callbackUrl      string
	repo             *repository.Store
}

type authOption func(*auth) error

func New(c Config, options ...authOption) (*auth, error) {
	a := &auth{
		log:              c.Log,
		tokenService:     c.TokenService,
		audit:            c.Auditing,
		providerBackends: map[string]providerBackend{},
		frontEndUrl:      c.FrontEndUrl,
		callbackUrl:      c.CallbackUrl,
		repo:             c.Repo,
	}
	return a.With(options...)
}

func (a *auth) NewHandler(isDevStage bool) (string, http.Handler, error) {
	a.log.Info("authhandler", "isDevStage", isDevStage)
	// FIXME: since go-1.22 and goth v1.81 can be replaced by
	// r := http.NewServeMux()
	r := mux.NewRouter()
	if isDevStage {
		_, err := a.With(FakeProvider())
		if err != nil {
			return "", nil, err
		}
	}

	key := []byte(os.Getenv("SESSION_SECRET"))
	gothic.Store = &sessions.CookieStore{
		Codecs: securecookie.CodecsFromPairs(key),
		Options: &sessions.Options{
			Path:     "/",
			MaxAge:   86400 * 30,
			SameSite: http.SameSiteLaxMode,
		},
	}

	// Register oauth login handler
	r.HandleFunc("/auth/{provider}/callback", a.Callback)
	// Register oauth login handler
	r.HandleFunc("/auth/{provider}", a.Login)
	// Register oauth logout handler
	r.HandleFunc("/auth/logout/{provider}", a.Logout)

	return "/auth/", r, nil
}

func (a *auth) With(options ...authOption) (*auth, error) {
	for _, o := range options {
		if err := o(a); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func (a *auth) AddProviderBackend(p providerBackend) {
	a.log.Info("add provider backend", "provider", p.Name())
	a.providerBackends[p.Name()] = p
}

func (a *auth) ProviderCallbackURL(provider string) string {
	return strings.Replace(a.callbackUrl, "{"+providerKey+"}", provider, 1)
}

type state struct {
	RedirectURL string `json:"r,omitempty"`
	Nonce       []byte `json:"n,omitempty"`
}

func newState(redirectURL string) (string, error) {
	if redirectURL != "" {
		u, err := url.Parse(redirectURL)
		if err != nil {
			return "", err
		}

		redirectURL = u.String()
	}

	nonce := make([]byte, 64)
	_, err := io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return "", fmt.Errorf("source of randomness is weird: %w", err)
	}

	s := &state{
		RedirectURL: redirectURL,
		Nonce:       nonce,
	}

	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(b), nil
}

func parseState(in string) (*state, error) {
	b, err := base64.URLEncoding.DecodeString(in)
	if err != nil {
		return nil, err
	}
	var o *state
	err = json.Unmarshal(b, &o)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (a *auth) Login(res http.ResponseWriter, req *http.Request) {
	// CheckLoggedIn
	// try to get the user without re-authenticating

	state, err := newState(req.URL.Query().Get("redirect-url"))
	if err != nil {
		http.Error(res, fmt.Sprintf("unable to set state: %v", err), http.StatusInternalServerError)
		return
	}
	a.log.Info("login", "state", state)

	q := req.URL.Query()
	q.Add("state", state)
	req.URL.RawQuery = q.Encode()

	gothUser, err := gothic.CompleteUserAuth(res, req)
	if err != nil {
		a.log.Info("no previous session found, restart auth workflow", "reason", err)
		gothic.BeginAuthHandler(res, req)
		return
	}

	a.log.Info("user login completed", "user", gothUser)
}

func (a *auth) Logout(res http.ResponseWriter, req *http.Request) {
	err := gothic.Logout(res, req)
	if err != nil {
		_, _ = fmt.Fprintln(res, err)
		return
	}

	providerName, err := gothic.GetProviderName(req)
	if err != nil {
		a.log.Error("cannot obtain provider name", "err", err)
	}
	provider, ok := a.providerBackends[providerName]

	if ok && provider.EndSessionRedirectURL() != "" {
		http.Redirect(res, req, provider.EndSessionRedirectURL(), http.StatusSeeOther)
		return
	}

	// FIXME invalidate token of this user.

	res.Header().Set("Location", "/static/")
	res.WriteHeader(http.StatusTemporaryRedirect)
}

func (a *auth) Callback(res http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	state, err := parseState(req.URL.Query().Get("state"))
	if err != nil {
		a.log.Error("unable to parse state", "err", err)
		http.Error(res, "unable to parse state", http.StatusInternalServerError)
		return
	}

	providerName, err := gothic.GetProviderName(req)
	if err != nil {
		a.log.Error("cannot obtain provider name", "err", err)
	}
	provider, ok := a.providerBackends[providerName]
	if !ok {
		a.log.Error("callback no provider backend found for", "provider", providerName)
		http.Error(res, "no provider backend", http.StatusBadRequest)
		return
	}
	user, err := gothic.CompleteUserAuth(res, req)
	if err != nil {
		a.log.Error("failed to complete user auth", "err", err)
		return
	}
	u, err := provider.User(ctx, user)
	if err != nil {
		http.Error(res, fmt.Sprintf("unable to extract user: %v", err), http.StatusUnauthorized)
		return
	}

	// Ensure tenant and token

	err = a.ensureTenant(ctx, u)
	if err != nil {
		http.Error(res, fmt.Sprintf("unable to create tenant: %v", err), http.StatusInternalServerError)
		return
	}

	// Create Token

	pat, err := a.repo.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(ctx, u.login)
	if err != nil {
		http.Error(res, fmt.Sprintf("unable to lookup projects and tenants: %v", err), http.StatusInternalServerError)
		return
	}

	a.log.Debug("callback", "project-roles", pat.ProjectRoles, "tenant-roles", pat.TenantRoles)

	// TODO: shall we create a user token in case a redirect url was given? Rename Console token to User token?

	tcr, err := a.tokenService.CreateConsoleTokenWithoutPermissionCheck(ctx, u.login, nil)
	if err != nil {
		http.Error(res, fmt.Sprintf("unable to create a token:%v", err), http.StatusInternalServerError)
		return
	}

	// unfortunately, the following does not work for all browsers (e.g. Firefox does not work):
	// redirectURL, err := url.Parse(req.Header.Get("Referer"))
	// if err != nil {
	// 	http.Error(res, fmt.Sprintf("no valid referer url: %v", err), http.StatusInternalServerError)
	// 	return
	// }
	rawQuery := url.Values{
		"token": []string{tcr.Msg.Secret},
	}.Encode()

	redirectURL := &url.URL{
		Scheme:   a.frontEndUrl.Scheme,
		Host:     a.frontEndUrl.Host,
		Path:     "login/auth",
		RawQuery: rawQuery,
	}
	if state.RedirectURL != "" {
		redirectURL, err = url.Parse(state.RedirectURL)
		if err != nil {
			a.log.Error("unable to parse redirect url from state", "error", err)
			http.Error(res, "unable to parse redirect url from state", http.StatusInternalServerError)
			return
		}

		redirectURL.RawQuery = rawQuery
	}

	if a.audit != nil {
		err = a.audit.Index(auditing.Entry{
			Component:    "auth",
			Type:         "login",
			User:         u.login,
			RemoteAddr:   req.RemoteAddr,
			ForwardedFor: req.Header.Get("X-Forwarded-For"),
			Body:         u,
		})
		if err != nil {
			a.log.Error("unable to index login request to audit backend", "error", err)
		}
	}

	a.log.Debug("redirecting back", "url", redirectURL.String())

	http.Redirect(res, req, redirectURL.String(), http.StatusSeeOther)
}

func (a *auth) ensureTenant(ctx context.Context, u *providerUser) error {
	tenant, err := a.repo.Tenant().Get(ctx, u.login)
	if err != nil && !errorutil.IsNotFound(err) {
		return fmt.Errorf("unable to get tenant %s: %w", u.login, err)
	}

	if err != nil && errorutil.IsNotFound(err) {
		created, err := a.repo.Tenant().AdditionalMethods().CreateWithID(ctx, &apiv2.TenantServiceCreateRequest{
			Name:      u.login,
			Email:     &u.email, // TODO: this field can be empty, fallback to user email would be great but for github this is also empty (#151)
			AvatarUrl: &u.avatarUrl,
		}, u.login)
		if err != nil {
			return fmt.Errorf("unable to create tenant:%s %w", u.login, err)
		}

		tenant = created
	}

	if tenant.Meta.Annotations[repository.TenantTagAvatarURL] != u.avatarUrl {
		tenant.Meta.Annotations[repository.TenantTagAvatarURL] = u.avatarUrl

		updated, err := a.repo.Tenant().Update(ctx, tenant.Meta.Id, &apiv2.TenantServiceUpdateRequest{
			Login:     u.login,
			AvatarUrl: &u.avatarUrl,
		})
		if err != nil {
			return fmt.Errorf("unable to update tenant:%s %w", u.login, err)
		}

		tenant = updated
	}

	_, err = a.repo.Tenant().AdditionalMethods().Member(u.login).Get(ctx, u.login)
	if err == nil {
		return nil
	}

	if errorutil.IsNotFound(err) {
		return err
	}

	_, err = a.repo.Tenant().AdditionalMethods().Member(u.login).Create(ctx, &repository.TenantMemberCreateRequest{
		Role:     apiv2.TenantRole_TENANT_ROLE_OWNER,
		MemberID: u.login,
	})

	return err
}
