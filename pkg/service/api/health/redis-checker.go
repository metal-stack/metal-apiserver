package health

import (
	"context"
	"fmt"
	"strings"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	valkeygo "github.com/valkey-io/valkey-go"
)

type redisHealthChecker struct {
	redis valkeygo.Client
}

func (h *redisHealthChecker) Health(ctx context.Context) *apiv2.HealthStatus {
	res, err := h.redis.Do(ctx, h.redis.B().Info().Section("server").Section("server_version").Build()).ToString()

	var (
		status  = apiv2.ServiceStatus_SERVICE_STATUS_HEALTHY
		message string
	)

	if err != nil {
		status = apiv2.ServiceStatus_SERVICE_STATUS_UNHEALTHY
		message = err.Error()
	} else {
		info := verbatimStringToMap(res)
		message = fmt.Sprintf("connected to redis service %q version %q", info["server_name"], info["redis_version"])
	}

	return &apiv2.HealthStatus{
		Name:    apiv2.Service_SERVICE_REDIS,
		Status:  status,
		Message: message,
	}
}

// implementation here is quite rough, but it did not make sense to put more effort into it
// would be nice if INFO would return something easier to parse
func verbatimStringToMap(s string) map[string]string {
	res := map[string]string{}

	for line := range strings.SplitSeq(s, "\r\n") {
		k, v, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found {
			continue
		}

		res[k] = v
	}

	return res
}
