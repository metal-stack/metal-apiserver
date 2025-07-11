package test

import (
	"log/slog"
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tlog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

const rethinkDbImage = "rethinkdb:2.4.4-bookworm-slim"

func StartRethink(t testing.TB, log *slog.Logger) (generic.Datastore, r.ConnectOpts, func()) {
	ctx := t.Context()

	req := testcontainers.ContainerRequest{
		Image:        rethinkDbImage,
		ExposedPorts: []string{"8080/tcp", "28015/tcp"},
		Env:          map[string]string{"RETHINKDB_PASSWORD": "rethink"},
		Tmpfs:        map[string]string{"/data": "rw"},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("28015/tcp"),
		),
		Cmd: []string{"rethinkdb", "--bind", "all", "--directory", "/data", "--initial-password", "rethink", "--io-threads", "500"},
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Logger:           tlog.TestLogger(t),
	})
	require.NoError(t, err)

	endpoint, err := c.PortEndpoint(ctx, "28015/tcp", "")
	require.NoError(t, err)

	opts := r.ConnectOpts{
		Address:  endpoint,
		Database: "metal",
		Username: "admin",
		Password: "rethink",
		MaxIdle:  10,
		MaxOpen:  20,
	}

	err = generic.Initialize(ctx, log, opts, generic.AsnPoolRange(uint(1), uint(100)), generic.VrfPoolRange(uint(1), uint(100)))
	require.NoError(t, err)

	ds, err := generic.New(log, opts)
	require.NoError(t, err)

	closer := func() {
		_ = c.Terminate(t.Context())
	}

	return ds, opts, closer
}
