package method

import (
	"context"
	"strings"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/api/go/permissions"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

type methodServiceServer struct {
	servicePermissions *permissions.ServicePermissions
}

func New() apiv2connect.MethodServiceHandler {
	servicePermissions := permissions.GetServicePermissions()

	return &methodServiceServer{
		servicePermissions: servicePermissions,
	}
}

func (m *methodServiceServer) List(ctx context.Context, _ *apiv2.MethodServiceListRequest) (*apiv2.MethodServiceListResponse, error) {
	token, ok := token.TokenFromContext(ctx)
	if !ok || token == nil {
		// only list public methods when there is no token

		var methods []string
		for m := range m.servicePermissions.Visibility.Public {
			methods = append(methods, m)
		}

		return &apiv2.MethodServiceListResponse{
			Methods: methods,
		}, nil
	}

	var (
		methods      []string
		isAdminToken = IsAdminToken(token)
	)
	for m := range m.servicePermissions.Methods {
		if isAdminToken {
			methods = append(methods, m)
			continue
		}

		if strings.HasPrefix(m, "/metalstack.api.v2") { // TODO: add all methods that do not require admin permissions
			methods = append(methods, m)
		}
	}

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
	}, nil
}
