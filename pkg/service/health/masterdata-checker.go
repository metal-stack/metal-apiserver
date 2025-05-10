package health

import (
	// ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	"context"
	"fmt"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"
)

type masterdataHealthChecker struct {
	mdm mdm.Client
}

func (h *masterdataHealthChecker) Health(ctx context.Context) *apiv2.HealthStatus {
	resp, err := h.mdm.Version().Get(ctx, &mdcv1.GetVersionRequest{})

	var (
		status  = apiv2.ServiceStatus_SERVICE_STATUS_HEALTHY
		message string
	)
	if err != nil {
		status = apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY
		message = err.Error()
	} else {
		message = fmt.Sprintf("connected to masterdata service version:%q", resp.Revision)
	}
	return &apiv2.HealthStatus{
		Name:    apiv2.Service_SERVICE_MASTERDATA,
		Status:  status,
		Message: message,
	}
}
