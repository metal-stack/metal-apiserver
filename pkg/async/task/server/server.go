package server

import (
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/redis/go-redis/v9"
)

func NewServer(log *slog.Logger, store *repository.Store, redis *redis.Client) (*asynq.Server, *asynq.ServeMux) {
	srv := asynq.NewServerFromRedisClient(
		redis,
		asynq.Config{
			Concurrency: 10,
			// Optionally specify multiple queues with different priority.
			// Queues: map[string]int{
			// 	"critical": 6,
			// 	"default":  3,
			// 	"low":      1,
			// },
		},
	)

	// mux maps a type to a handler

	mux := asynq.NewServeMux()
	mux.HandleFunc(string(task.TypeIpDelete), store.IpDeleteHandleFn)
	mux.HandleFunc(string(task.TypeNetworkDelete), store.NetworkDeleteHandleFn)
	mux.HandleFunc(string(task.TypeMachineDelete), store.MachineDeleteHandleFn)
	mux.HandleFunc(string(task.TypeMachineBMCCommand), store.MachineBMCCommandHandleFn)

	// ...register other handlers...
	return srv, mux
}
