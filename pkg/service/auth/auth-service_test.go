package auth

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

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
			redirectUrls: []string{"http://localhost", "https://localhost", "https://metal-stack.io","http://127.0.0.1"},
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
