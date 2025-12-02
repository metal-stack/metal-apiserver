package request

import (
	"context"
	"log/slog"
	"testing"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/stretchr/testify/require"
)

func Benchmark_allow(b *testing.B) {
	a := &authorizer{
		log: slog.Default(),
	}
	a.projectsAndTenantsGetter = func(ctx context.Context, userId string) (*repository.ProjectsAndTenants, error) {
		return &repository.ProjectsAndTenants{
			ProjectRoles: map[string]apiv2.ProjectRole{
				"project-a": apiv2.ProjectRole_PROJECT_ROLE_EDITOR,
			},
		}, nil
	}

	for b.Loop() {
		token := &apiv2.Token{
			User:      "user-a",
			TokenType: apiv2.TokenType_TOKEN_TYPE_API,
			Permissions: []*apiv2.MethodPermission{
				{Subject: "project-a", Methods: []string{"/metalstack.api.v2.IPService/Get"}},
			},
		}
		message := "/metalstack.api.v2.IPService/Get"
		subject := "project-a"
		err := a.authorize(b.Context(), token, message, subject)
		require.NoError(b, err)
	}
}
