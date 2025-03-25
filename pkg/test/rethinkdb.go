package test

import (
	"log/slog"
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

const rethinkDbImage = "rethinkdb:2.4.4-bookworm-slim"

func StartRethink(t testing.TB, log *slog.Logger) (container testcontainers.Container, s r.ConnectOpts, err error) {
	ctx := t.Context()
	var tLog testcontainers.Logging
	if t != nil {
		tLog = testcontainers.TestLogger(t)
	}

	req := testcontainers.ContainerRequest{
		Image:        rethinkDbImage,
		ExposedPorts: []string{"8080/tcp", "28015/tcp"},
		Env:          map[string]string{"RETHINKDB_PASSWORD": "rethink"},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("28015/tcp"),
		),
		Cmd: []string{"rethinkdb", "--bind", "all", "--directory", "/tmp", "--initial-password", "rethink", "--io-threads", "500"},
	}

	rtContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Logger:           tLog,
	})
	require.NoError(t, err)

	ip, err := rtContainer.Host(ctx)
	require.NoError(t, err)

	port, err := rtContainer.MappedPort(ctx, "28015")
	require.NoError(t, err)

	opts := r.ConnectOpts{
		Addresses: []string{ip + ":" + port.Port()},
		Database:  "metal",
		Username:  "admin",
		Password:  "rethink",
		MaxIdle:   10,
		MaxOpen:   20,
	}

	err = generic.Initialize(ctx, log, opts)
	require.NoError(t, err)

	return rtContainer, opts, err
}
