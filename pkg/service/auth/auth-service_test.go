package auth

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type testEndSessionProvider struct {
	fakeProviderBackend
	endSessionURL string
}

func (p *testEndSessionProvider) EndSessionRedirectURL() string {
	return p.endSessionURL
}

func newTestAuth(t *testing.T) *auth {
	t.Helper()

	t.Setenv("SESSION_SECRET", "test-session-secret")

	a, err := New(Config{
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		CallbackUrl: "http://localhost/auth/{provider}/callback",
	})
	require.NoError(t, err)

	_, err = a.With(FakeProvider())
	require.NoError(t, err)

	return a
}

func TestNewHandler_routesProviderRequests(t *testing.T) {
	tests := []struct {
		name             string
		method           string
		path             string
		wantStatus       int
		wantLocation     string
		wantLocationPart string
	}{
		{
			name:             "login starts the auth workflow for the provider from the path",
			method:           http.MethodGet,
			path:             "/auth/test",
			wantStatus:       http.StatusTemporaryRedirect,
			wantLocationPart: "/auth/test/callback?state=",
		},
		{
			name:         "logout redirects to the provider end session url",
			method:       http.MethodGet,
			path:         "/auth/logout/test",
			wantStatus:   http.StatusSeeOther,
			wantLocation: "https://idp.example/logout",
		},
		{
			name:       "callback for the provider from the path is invoked",
			method:     http.MethodGet,
			path:       "/auth/test/callback?state=not-valid-base64",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "unknown sub path is not handled",
			method:     http.MethodGet,
			path:       "/auth/test/unknown",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "nested callback path is not handled",
			method:     http.MethodGet,
			path:       "/auth/test/callback/extra",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAuth(t)
			a.providerBackends["test"] = &testEndSessionProvider{
				endSessionURL: "https://idp.example/logout",
			}

			path, handler, err := a.NewHandler()
			require.NoError(t, err)
			require.Equal(t, "/auth/", path)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			res := httptest.NewRecorder()

			handler.ServeHTTP(res, req)

			require.Equal(t, tt.wantStatus, res.Code)
			if tt.wantLocation != "" {
				require.Equal(t, tt.wantLocation, res.Header().Get("Location"))
			}
			if tt.wantLocationPart != "" {
				require.Contains(t, res.Header().Get("Location"), tt.wantLocationPart)
			}
		})
	}
}

func TestNewHandler_canBeMountedOnAServeMux(t *testing.T) {
	a := newTestAuth(t)

	path, handler, err := a.NewHandler()
	require.NoError(t, err)

	root := http.NewServeMux()
	root.Handle(path, handler)

	req := httptest.NewRequest(http.MethodGet, "/auth/test", nil)
	res := httptest.NewRecorder()

	root.ServeHTTP(res, req)

	require.Equal(t, http.StatusTemporaryRedirect, res.Code)
	require.True(t, strings.HasPrefix(res.Header().Get("Location"), "/auth/test/callback?state="))
}

func Test_auth_isRedirectURLAllowed(t *testing.T) {
	tests := []struct {
		name         string
		redirectUrls []string
		url          string
		wantErr      bool
	}{
		{
			name:         "url is allowed",
			redirectUrls: []string{"http://localhost", "https://localhost", "https://metal-stack.io"},
			url:          "http://localhost:8080/login?token=asdf",
			wantErr:      false,
		},
		{
			name:         "url with ip is allowed",
			redirectUrls: []string{"http://localhost", "https://localhost", "https://metal-stack.io", "http://127.0.0.1"},
			url:          "http://127.0.0.1:8080/login?token=asdf",
			wantErr:      false,
		},
		{
			name:         "url is not allowed",
			redirectUrls: []string{"http://localhost", "https://localhost", "https://metal-stack.io"},
			url:          "http://evil.com:8080",
			wantErr:      true,
		},
		{
			name:         "url is not allowed",
			redirectUrls: []string{"https://localhost", "https://metal-stack.io"},
			url:          "http://localhost:8080/login?token=asdf",
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				redirectUrls []*url.URL
			)

			requestedUrl, err := url.Parse(tt.url)
			require.NoError(t, err)

			for _, u := range tt.redirectUrls {
				parsed, err := url.Parse(u)
				require.NoError(t, err)
				redirectUrls = append(redirectUrls, parsed)
			}
			a := auth{
				redirectUrls: redirectUrls,
			}

			gotErr := a.isRedirectURLAllowed(requestedUrl)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("isRedirectURLAllowed() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("isRedirectURLAllowed() succeeded unexpectedly")
			}
		})
	}
}
