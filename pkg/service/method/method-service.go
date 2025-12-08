package method

import (
	"context"
	"log/slog"
	"slices"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/api/go/permissions"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/request"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/cache"
)

type methodServiceServer struct {
	log                *slog.Logger
	authorizer         request.Authorizer
	servicePermissions *permissions.ServicePermissions
}

func New(log *slog.Logger, repo *repository.Store) apiv2connect.MethodServiceHandler {
	projectAndTenantCache := cache.New(10*time.Second, func(ctx context.Context, id string) (*repository.ProjectsAndTenants, error) {
		pat, err := repo.UnscopedProject().AdditionalMethods().GetProjectsAndTenants(ctx, id)
		if err != nil {
			return nil, err
		}
		return pat, nil
	})

	patg := func(ctx context.Context, userId string) (*repository.ProjectsAndTenants, error) {
		return projectAndTenantCache.Get(ctx, userId)
	}

	return &methodServiceServer{
		log:                log,
		servicePermissions: permissions.GetServicePermissions(),
		authorizer:         request.NewAuthorizer(log, patg),
	}
}

// List return the effective list of methods accessible with the given token.
// All methods can already be calculated on the client side.
func (m *methodServiceServer) List(ctx context.Context, _ *apiv2.MethodServiceListRequest) (*apiv2.MethodServiceListResponse, error) {
	token, _ := token.TokenFromContext(ctx)

	permissions, err := m.authorizer.TokenPermissions(ctx, token)
	if err != nil {
		return nil, err
	}

	var methods []string
	for method := range permissions {
		methods = append(methods, method)
	}
	slices.Sort(methods)

	return &apiv2.MethodServiceListResponse{
		Methods: methods,
	}, nil
}

func (m *methodServiceServer) TokenScopedList(ctx context.Context, _ *apiv2.MethodServiceTokenScopedListRequest) (*apiv2.MethodServiceTokenScopedListResponse, error) {
	token, ok := token.TokenFromContext(ctx)
	if !ok || token == nil {
		return nil, errorutil.Unauthenticated("no token found in request")
	}

	return &apiv2.MethodServiceTokenScopedListResponse{
		Permissions:  token.Permissions,
		ProjectRoles: token.ProjectRoles,
		TenantRoles:  token.TenantRoles,
		AdminRole:    token.AdminRole,
		InfraRole:    token.InfraRole,
	}, nil
}
