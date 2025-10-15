package health

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	apiv1 "github.com/metal-stack/go-ipam/api/v1"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
)

type ipamHealthChecker struct {
	ipam ipamv1connect.IpamServiceClient
}

func (h *ipamHealthChecker) Health(ctx context.Context) *apiv2.HealthStatus {
	resp, err := h.ipam.Version(ctx, connect.NewRequest(&apiv1.VersionRequest{}))

	var (
		status  = apiv2.ServiceStatus_SERVICE_STATUS_HEALTHY
		message string
	)

	if err != nil {
		status = apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY
		message = err.Error()
	} else {
		message = fmt.Sprintf("connected to ipam service version %q", resp.Msg.Revision)
	}

	return &apiv2.HealthStatus{
		Name:    apiv2.Service_SERVICE_IPAM,
		Status:  status,
		Message: message,
	}
}
