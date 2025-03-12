package server

import (
	"log/slog"

	"github.com/hibiken/asynq"
	asyncclient "github.com/metal-stack/metal-apiserver/pkg/async/client"
	"github.com/metal-stack/metal-apiserver/pkg/db/repository"
	"github.com/redis/go-redis/v9"
)

func New(log *slog.Logger, store *repository.Store, redis *redis.Client) (*asynq.Server, *asynq.ServeMux) {
	srv := asynq.NewServerFromRedisClient(
		redis,
		asynq.Config{
			// Specify how many concurrent workers to use
			Concurrency: 10,
			// Optionally specify multiple queues with different priority.
			// Queues: map[string]int{
			// 	"critical": 6,
			// 	"default":  3,
			// 	"low":      1,
			// },
			// See the godoc for other configuration options
		},
	)

	// mux maps a type to a handler

	mux := asynq.NewServeMux()
	mux.HandleFunc(asyncclient.TypeIpDelete, store.IpDeleteHandleFn)
	// ...register other handlers...
	return srv, mux
}
