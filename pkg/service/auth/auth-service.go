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
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/gorilla/mux"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdc "github.com/metal-stack/masterdata-api/pkg/client"
	"google.golang.org/protobuf/types/known/wrapperspb"

	putil "github.com/metal-stack/api-server/pkg/project"
	"github.com/metal-stack/api-server/pkg/service/token"
	tutil "github.com/metal-stack/api-server/pkg/tenant"
	"github.com/metal-stack/metal-lib/auditing"
)

const (
	providerKey = "provider"
)

type Config struct {
	TokenService token.TokenService
	MasterClient mdc.Client
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
	masterClient     mdc.Client
	audit            auditing.Auditing
	log              *slog.Logger
	frontEndUrl      *url.URL
	callbackUrl      string
}

type authOption func(*auth)

func New(c Config, options ...authOption) *auth {
	a := &auth{
		log:              c.Log,
		tokenService:     c.TokenService,
		masterClient:     c.MasterClient,
		audit:            c.Auditing,
		providerBackends: map[string]providerBackend{},
		frontEndUrl:      c.FrontEndUrl,
		callbackUrl:      c.CallbackUrl,
	}
	return a.With(options...)
}

func (a *auth) NewHandler(isDevStage bool) (string, http.Handler) {
	a.log.Info("authhandler", "isDevStage", isDevStage)
	// FIXME: can be replaced byr  := http.NewServeMux() since go-1.22
	r := mux.NewRouter()
	if isDevStage {
		a.With(FakeProvider())
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

	return "/auth/", r
}

func (a *auth) With(options ...authOption) *auth {
	for _, o := range options {
		o(a)
	}
	return a
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
		fmt.Fprintln(res, err)
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

	// Ensure tenant and default project and token

	err = a.ensureTenant(ctx, u)
	if err != nil {
		http.Error(res, fmt.Sprintf("unable to create tenant: %v", err), http.StatusInternalServerError)
		return
	}

	err = a.ensureDefaultProject(ctx, u)
	if err != nil {
		http.Error(res, fmt.Sprintf("unable to create tenant: %v", err), http.StatusInternalServerError)
		return
	}

	// Create Token

	pat, err := putil.GetProjectsAndTenants(ctx, a.masterClient, u.login, putil.DefaultProjectRequired)
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

// FIXME move to repository
func (a *auth) ensureTenant(ctx context.Context, u *providerUser) error {
	resp, err := a.masterClient.Tenant().Get(ctx, &mdcv1.TenantGetRequest{
		Id: u.login,
	})
	if err != nil && !mdcv1.IsNotFound(err) {
		return fmt.Errorf("unable to get tenant:%s %w", u.login, err)
	}

	var tenant *mdcv1.Tenant

	if err != nil && mdcv1.IsNotFound(err) {
		resp, err := a.masterClient.Tenant().Create(ctx, &mdcv1.TenantCreateRequest{
			Tenant: &mdcv1.Tenant{
				Meta: &mdcv1.Meta{
					Id: u.login,
					Annotations: map[string]string{
						tutil.TagEmail:     u.email, // TODO: this field can be empty, fallback to user email would be great but for github this is also empty (#151)
						tutil.TagAvatarURL: u.avatarUrl,
						tutil.TagCreator:   u.login,
					},
				},
				Name: u.name,
			},
		})
		if err != nil {
			return fmt.Errorf("unable to create tenant:%s %w", u.login, err)
		}

		tenant = resp.Tenant
	} else {
		tenant = resp.Tenant
	}

	if tenant.Meta.Annotations[tutil.TagAvatarURL] != u.avatarUrl {
		tenant.Meta.Annotations[tutil.TagAvatarURL] = u.avatarUrl

		_, err = a.masterClient.Tenant().Update(ctx, &mdcv1.TenantUpdateRequest{
			Tenant: tenant,
		})
		if err != nil {
			return fmt.Errorf("unable to update tenant:%s %w", u.login, err)
		}
	}

	_, err = tutil.GetTenantMember(ctx, a.masterClient, u.login, u.login)
	if err == nil {
		return nil
	}

	if connect.CodeOf(err) != connect.CodeNotFound {
		return err
	}

	_, err = a.masterClient.TenantMember().Create(ctx, &mdcv1.TenantMemberCreateRequest{
		TenantMember: &mdcv1.TenantMember{
			Meta: &mdcv1.Meta{
				Annotations: map[string]string{
					tutil.TenantRoleAnnotation: apiv2.TenantRole_TENANT_ROLE_OWNER.String(),
				},
			},
			TenantId: u.login,
			MemberId: u.login,
		},
	})

	return err
}

// FIXME move to repository
func (a *auth) ensureDefaultProject(ctx context.Context, u *providerUser) error {
	ensureMembership := func(projectId string) error {
		_, _, err := putil.GetProjectMember(ctx, a.masterClient, projectId, u.login)
		if err == nil {
			return nil
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			return err
		}

		_, err = a.masterClient.ProjectMember().Create(ctx, &mdcv1.ProjectMemberCreateRequest{
			ProjectMember: &mdcv1.ProjectMember{
				Meta: &mdcv1.Meta{
					Annotations: map[string]string{
						putil.ProjectRoleAnnotation: apiv2.ProjectRole_PROJECT_ROLE_OWNER.String(),
					},
				},
				ProjectId: projectId,
				TenantId:  u.login,
			},
		})

		return err
	}

	resp, err := a.masterClient.Project().Find(ctx, &mdcv1.ProjectFindRequest{
		TenantId: wrapperspb.String(u.login),
		Annotations: map[string]string{
			putil.DefaultProjectAnnotation: strconv.FormatBool(true),
		},
	})
	if err != nil {
		return fmt.Errorf("unable to get find projects: %w", err)
	}

	if len(resp.Projects) > 0 {
		return ensureMembership(resp.Projects[0].Meta.Id)
	}

	project, err := a.masterClient.Project().Create(ctx, &mdcv1.ProjectCreateRequest{
		Project: &mdcv1.Project{
			Meta: &mdcv1.Meta{
				Annotations: map[string]string{
					putil.DefaultProjectAnnotation: strconv.FormatBool(true),
				},
			},
			Name:        "Default Project",
			TenantId:    u.login,
			Description: "Default project of " + u.login,
		},
	})
	if err != nil {
		return fmt.Errorf("unable to create project: %w", err)
	}

	return ensureMembership(project.Project.Meta.Id)
}
