package auth

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/openidConnect"
)

type provider struct {
	log *slog.Logger
}

type ProviderConfig struct {
	ClientID     string
	ClientSecret string
	DiscoveryURL string
}

func OIDCHubProvider(c ProviderConfig) authOption {
	return func(a *auth) {
		if c.ClientID == "" || c.ClientSecret == "" {
			a.log.Warn("no oidc client id or secret configured")
			return
		}
		p := &provider{
			log: a.log,
		}
		scopes := []string{"openid", "email", "profile"}
		// FIXME check error
		oidc, _ := openidConnect.New(
			c.ClientID,
			c.ClientSecret,
			a.ProviderCallbackURL(p.Name()),
			c.DiscoveryURL,
			scopes...,
		)
		oidc.SetName(p.Name())
		goth.UseProviders(oidc)
		a.AddProviderBackend(p)
	}
}

func (g *provider) Name() string {
	return "oidc"
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
