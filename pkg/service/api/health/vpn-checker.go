package health

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/headscale"
)

type vpnHealthChecker struct {
	client *headscale.Client
}

func (h *vpnHealthChecker) Health(ctx context.Context) *apiv2.HealthStatus {
	// TODO: can be added back with headscale >= v0.27
	// res, err := h.client.Health(ctx, &headscalev1.HealthRequest{})

	// var (
	// 	status  = apiv2.ServiceStatus_SERVICE_STATUS_HEALTHY
	// 	message string
	// )

	// if err != nil {
	// 	status = apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY
	// 	message = err.Error()
	// } else if !res.DatabaseConnectivity {
	// 	status = apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY
	// 	message = "no connection to database"
	// } else {
	// 	message = "connected to vpn service"
	// }

	// return &apiv2.HealthStatus{
	// 	Name:    apiv2.Service_SERVICE_VPN,
	// 	Status:  status,
	// 	Message: message,
	// }

	return &apiv2.HealthStatus{
		Name:    apiv2.Service_SERVICE_VPN,
		Status:  apiv2.ServiceStatus_SERVICE_STATUS_HEALTHY,
		Message: "",
	}
}
