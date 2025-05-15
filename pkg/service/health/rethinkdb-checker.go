package health

import (
	"context"
	"fmt"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
)

type rethinkdbHealthChecker struct {
	ds generic.Datastore
}

func (h *rethinkdbHealthChecker) Health(ctx context.Context) *apiv2.HealthStatus {
	version, err := h.ds.Version(ctx)
	if err != nil {
		return &apiv2.HealthStatus{
			Name:    apiv2.Service_SERVICE_RETHINK,
			Status:  apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY,
			Message: fmt.Sprintf("unable to select version from rethinkdb: %v", err),
		}
	}

	return &apiv2.HealthStatus{
		Name:    apiv2.Service_SERVICE_RETHINK,
		Status:  apiv2.ServiceStatus_SERVICE_STATUS_HEALTHY,
		Message: fmt.Sprintf("connected to rethinkdb version %q", version),
	}
}
