package auth

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/openidConnect"
)

type provider struct {
	log *slog.Logger
	pc  ProviderConfig
}

type ProviderConfig struct {
	ClientID      string
	ClientSecret  string
	DiscoveryURL  string
	EndsessionURL string
}

func OIDCHubProvider(c ProviderConfig) authOption {
	return func(a *auth) error {
		if c.ClientID == "" || c.ClientSecret == "" {
			return fmt.Errorf("oidc client id or secret is not configured")
		}
		p := &provider{
			log: a.log,
			pc:  c,
		}
		scopes := []string{"openid", "email", "profile"}

		tlsConf := &tls.Config{
			InsecureSkipVerify: true,
		}

		oidc, err := openidConnect.NewCustomisedHttpClient(
			&http.Client{Transport: &http.Transport{TLSClientConfig: tlsConf}},
			p.Name(),
			c.ClientID,
			c.ClientSecret,
			a.ProviderCallbackURL(p.Name()),
			c.DiscoveryURL,
			scopes...,
		)
		if err != nil {
			return fmt.Errorf("unable to initialize oidc provider: %w", err)
		}

		goth.UseProviders(oidc)
		a.AddProviderBackend(p)

		a.log.Info("configured oidc provider", "provider", p.Name())

		return nil
	}
}

func (g *provider) Name() string {
	return "oidc"
}

func (g *provider) EndSessionRedirectURL() string {
	return g.pc.EndsessionURL
}

func (g *provider) User(ctx context.Context, user goth.User) (*providerUser, error) {
	g.log.Info("user", "user", user)
	sub, ok := user.RawData["sub"]
	if !ok {
		return nil, fmt.Errorf("oidc raw data does not contain sub field")
	}

	login, ok := sub.(string)
	if !ok {
		return nil, fmt.Errorf("oidc login field does not contain a string (but %T)", sub)
	}

	return &providerUser{
		login:     g.getLogin(login),
		name:      user.Name,
		email:     user.Email,
		avatarUrl: user.AvatarURL,
	}, nil
}

func (g *provider) getLogin(s string) string {
	return s + "@" + g.Name()
}
