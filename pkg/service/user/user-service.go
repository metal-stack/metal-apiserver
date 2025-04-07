package user

import (
	"context"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type userServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(config *Config) apiv2connect.UserServiceHandler {
	return &userServiceServer{
		log:  config.Log,
		repo: config.Repo,
	}
}

func (u *userServiceServer) Get(ctx context.Context, _ *connect.Request[apiv2.UserServiceGetRequest]) (*connect.Response[apiv2.UserServiceGetResponse], error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)

	if !ok || t == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("no token found in request"))
	}

	projectsAndTenants, err := u.repo.UnscopedProject().GetProjectsAndTenants(ctx, t.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	user := &apiv2.User{
		Login:          t.UserId,
		Name:           projectsAndTenants.DefaultTenant.Name,
		Email:          projectsAndTenants.DefaultTenant.Email,
		AvatarUrl:      projectsAndTenants.DefaultTenant.AvatarUrl,
		Tenants:        projectsAndTenants.Tenants,
		Projects:       projectsAndTenants.Projects,
		DefaultTenant:  projectsAndTenants.DefaultTenant,
		DefaultProject: projectsAndTenants.DefaultProject,
	}

	return connect.NewResponse(&apiv2.UserServiceGetResponse{User: user}), nil
}
