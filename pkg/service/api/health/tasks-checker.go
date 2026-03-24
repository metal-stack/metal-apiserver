package health

import (
	"context"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
)

type tasksHealthChecker struct {
	tasks *task.Client
}

func (h *tasksHealthChecker) Health(ctx context.Context) *apiv2.HealthStatus {
	err := h.tasks.Ping()

	var (
		status  = apiv2.ServiceStatus_SERVICE_STATUS_HEALTHY
		message string
	)

	if err != nil {
		status = apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY
		message = err.Error()
	} else {
		message = "connected to tasks server"
	}

	return &apiv2.HealthStatus{
		Name:    apiv2.Service_SERVICE_TASKS,
		Status:  status,
		Message: message,
	}
}
