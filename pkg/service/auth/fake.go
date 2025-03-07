package auth

import (
	"context"

	"github.com/markbates/goth"
	"golang.org/x/oauth2"
)

type fakeProviderBackend struct{}

type fakeSession struct {
	state string
}

func (f fakeSession) GetAuthURL() (string, error) {
	return "/auth/test/callback?state=" + f.state, nil
}

func (f fakeSession) Marshal() string {
	return f.state
}

func (f fakeSession) Authorize(_ goth.Provider, _ goth.Params) (string, error) {
	return "", nil
}

func FakeProvider() authOption {
	return func(a *auth) {
		p := &fakeProviderBackend{}
		goth.UseProviders(p)
		a.AddProviderBackend(p)
	}
}

func (p *fakeProviderBackend) Name() string {
	return "test"
}

func (p *fakeProviderBackend) SetName(_ string) {}

func (p *fakeProviderBackend) BeginAuth(state string) (goth.Session, error) {
	return &fakeSession{
		state: state,
	}, nil
}

func (p *fakeProviderBackend) UnmarshalSession(s string) (goth.Session, error) {
	return fakeSession{
		state: s,
	}, nil
}

func (p *fakeProviderBackend) FetchUser(_ goth.Session) (goth.User, error) {
	return goth.User{}, nil
}

func (p *fakeProviderBackend) Debug(_ bool) {}

func (p *fakeProviderBackend) RefreshToken(_ string) (*oauth2.Token, error) {
	panic("not implemented")
}

func (p *fakeProviderBackend) RefreshTokenAvailable() bool {
	return false
}

func (p *fakeProviderBackend) User(_ context.Context, _ goth.User) (*providerUser, error) {
	return &providerUser{
		login:     "Testman@test",
		name:      "John Test",
		email:     "Testman",
		avatarUrl: "https://avatars.githubusercontent.com/u/101409188?v=4",
		provider:  p.Name(),
	}, nil
}
