package health

import (
	"context"
	"log/slog"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/auditing"
	"github.com/metal-stack/metal-lib/pkg/healthstatus"
)

type auditHealthChecker struct {
	log      *slog.Logger
	backends []auditing.Auditing
}

func (h *auditHealthChecker) Health(ctx context.Context) *apiv2.HealthStatus {
	var checks []healthstatus.HealthCheck

	for _, backend := range h.backends {
		checks = append(checks, backend)
	}

	res, err := healthstatus.Grouped(h.log, "audit", checks...).Check(ctx)

	var (
		status  = apiv2.ServiceStatus_SERVICE_STATUS_HEALTHY
		message string
	)

	if err != nil {
		status = apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY
		message = res.Message
	} else {
		switch res.Status {
		case healthstatus.HealthStatusHealthy:
			// noop
		case healthstatus.HealthStatusDegraded, healthstatus.HealthStatusPartiallyUnhealthy:
			status = apiv2.ServiceStatus_SERVICE_STATUS_DEGRADED
		case healthstatus.HealthStatusUnhealthy:
			status = apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY
		}

		message = res.Message
	}

	return &apiv2.HealthStatus{
		Name:    apiv2.Service_SERVICE_AUDIT,
		Status:  status,
		Message: message,
	}
}
