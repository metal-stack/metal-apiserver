package health

import (
	"context"
	"fmt"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	tenantv1 "github.com/metal-stack/tenant-api/go/api/v1"
	tenant "github.com/metal-stack/tenant-api/go/client"
)

type tenantApiserverHealthChecker struct {
	tenant tenant.Client
}

func (h *tenantApiserverHealthChecker) Health(ctx context.Context) *apiv2.HealthStatus {
	resp, err := h.tenant.Apiv1().Version().Get(ctx, &tenantv1.VersionServiceGetRequest{})

	var (
		status  = apiv2.ServiceStatus_SERVICE_STATUS_HEALTHY
		message string
	)

	if err != nil {
		status = apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY
		message = err.Error()
	} else {
		message = fmt.Sprintf("connected to tenant-apiserver service version %q", resp.Revision)
	}

	return &apiv2.HealthStatus{
		Name:    apiv2.Service_SERVICE_TENANT_APISERVER,
		Status:  status,
		Message: message,
	}
}
