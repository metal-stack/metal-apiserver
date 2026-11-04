package health

import (
	"context"
	"log/slog"

	headscalev1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/headscale"
)

type vpnHealthChecker struct {
	log    *slog.Logger
	client *headscale.Client
}

func (h *vpnHealthChecker) Health(ctx context.Context) *apiv2.HealthStatus {
	var (
		status  = apiv2.ServiceStatus_SERVICE_STATUS_HEALTHY
		version = "unknown"
		message string
	)
	versionString, err := h.client.Version(ctx)
	if err != nil {
		h.log.Error("error fetching version", "error", err)
	} else {
		version = versionString
	}

	res, err := h.client.Health(ctx, &headscalev1.HealthRequest{})
	if err != nil {
		status = apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY
		message = err.Error()
	} else if !res.DatabaseConnectivity {
		status = apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY
		message = "no connection to database"
	} else {
		message = "connected to vpn service, version: " + version
	}

	return &apiv2.HealthStatus{
		Name:    apiv2.Service_SERVICE_VPN,
		Status:  status,
		Message: message,
	}
}
