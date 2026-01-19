//go:build export
// +build export

package test

import (
	"log/slog"
	"testing"

	asyncserver "github.com/metal-stack/metal-apiserver/pkg/async/server"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func StartAsynqServer(t testing.TB, log *slog.Logger, repository *repository.Store, redis *redis.Client) func() {
	asyncServer, asyncServerMux := asyncserver.New(log, repository, redis)
	go func() {
		log.Info("starting asynq server")
		err := asyncServer.Run(asyncServerMux)
		assert.NoError(t, err)
	}()

	closer := func() {
		asyncServer.Shutdown()
	}
	return closer
}
