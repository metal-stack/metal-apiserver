package user

import (
	"context"
	"log/slog"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
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

func (u *userServiceServer) Get(ctx context.Context, _ *apiv2.UserServiceGetRequest) (*apiv2.UserServiceGetResponse, error) {
	var (
		t, ok = token.TokenFromContext(ctx)
	)

	if !ok || t == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	projectsAndTenants, err := u.repo.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(ctx, t.User)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	user := &apiv2.User{
		Login:         t.User,
		Name:          projectsAndTenants.DefaultTenant.Name,
		Email:         projectsAndTenants.DefaultTenant.Email,
		AvatarUrl:     projectsAndTenants.DefaultTenant.AvatarUrl,
		Tenants:       projectsAndTenants.Tenants,
		Projects:      projectsAndTenants.Projects,
		DefaultTenant: projectsAndTenants.DefaultTenant,
	}

	return &apiv2.UserServiceGetResponse{User: user}, nil
}
