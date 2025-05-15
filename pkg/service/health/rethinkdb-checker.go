package health

import (
	"context"
	"fmt"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

type rethinkdbHealthChecker struct {
	connectOpts r.ConnectOpts
}

func (h *rethinkdbHealthChecker) Health(ctx context.Context) *apiv2.HealthStatus {
	var (
		version string
		status  = apiv2.ServiceStatus_SERVICE_STATUS_HEALTHY
		message string
	)

	session, err := r.Connect(h.connectOpts)
	if err != nil {
		return &apiv2.HealthStatus{
			Name:    apiv2.Service_SERVICE_RETHINK,
			Status:  apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY,
			Message: fmt.Sprintf("unable to connect to rethinkdb: %v", err),
		}
	}

	cursor, err := r.DB("rethinkdb").Table("server_status").Field("process").Field("version").Run(session, r.RunOpts{Context: ctx})
	if err != nil {
		return &apiv2.HealthStatus{
			Name:    apiv2.Service_SERVICE_RETHINK,
			Status:  apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY,
			Message: fmt.Sprintf("unable to select version from rethinkdb: %v", err),
		}
	}

	err = cursor.One(&version)
	if err != nil {
		status = apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY
		message = err.Error()
	} else {
		message = fmt.Sprintf("connected to rethinkdb version %q", version)
	}

	return &apiv2.HealthStatus{
		Name:    apiv2.Service_SERVICE_RETHINK,
		Status:  status,
		Message: message,
	}
}
