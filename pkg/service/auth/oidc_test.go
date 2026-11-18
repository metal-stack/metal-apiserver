package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_resolveScopes(t *testing.T) {
	tests := []struct {
		name string
		user []string
		want []string
	}{
		{
			name: "no user scopes returns defaults",
			user: nil,
			want: []string{"openid", "email", "profile"},
		},
		{
			name: "user scopes are appended to defaults",
			user: []string{"groups", "roles"},
			want: []string{"openid", "email", "profile", "groups", "roles"},
		},
		{
			name: "duplicates of defaults are not repeated",
			user: []string{"email", "openid", "profile"},
			want: []string{"openid", "email", "profile"},
		},
		{
			name: "duplicate user scopes appear once",
			user: []string{"groups", "groups"},
			want: []string{"openid", "email", "profile", "groups"},
		},
		{
			name: "whitespace is trimmed",
			user: []string{" groups ", "\troles\t"},
			want: []string{"openid", "email", "profile", "groups", "roles"},
		},
		{
			name: "empty entries are dropped",
			user: []string{"", "  ", "groups"},
			want: []string{"openid", "email", "profile", "groups"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveScopes(tt.user)
			require.Equal(t, tt.want, got)
		})
	}
}
