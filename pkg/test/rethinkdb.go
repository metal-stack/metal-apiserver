package test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tlog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

const rethinkDbImage = "rethinkdb:2.4.4-bookworm-slim"

var (
	connectOpts        r.ConnectOpts
	rethinkDbEndpoint  string
	closer             func()
	mtx                sync.Mutex
	rethinkdbContainer testcontainers.Container
)

func StartRethink(t testing.TB, log *slog.Logger) (generic.Datastore, r.ConnectOpts, func()) {
	mtx.Lock()
	defer mtx.Unlock()

	if rethinkDbEndpoint == "" {
		ctx := context.Background()

		c, err := testcontainers.Run(
			ctx,
			rethinkDbImage,
			testcontainers.WithExposedPorts("8080/tcp", "28015/tcp"),
			testcontainers.WithTmpfs(map[string]string{"/data": "rw"}),
			testcontainers.WithWaitStrategy(
				wait.ForListeningPort("28015/tcp").WithStartupTimeout(time.Second*5),
				wait.ForExposedPort(),
			),
			testcontainers.WithEnv(map[string]string{"RETHINKDB_PASSWORD": "rethink"}),
			testcontainers.WithCmd("rethinkdb", "--bind", "all", "--directory", "/data", "--initial-password", "rethink", "--io-threads", "500"),
			testcontainers.WithLogger(tlog.TestLogger(t)),
		)
		require.NoError(t, err)

		rethinkDbEndpoint, err = c.PortEndpoint(ctx, "28015/tcp", "")
		require.NoError(t, err)

		rethinkdbContainer = c
		closer = func() {
			// TODO: clean up database of this test

			// we do not terminate the container here because it's very complex with a shared ds
			// testcontainers will cleanup the database by itself
		}
	}

	connectOpts = r.ConnectOpts{
		Address:  rethinkDbEndpoint,
		Database: databaseNameFromT(t),
		Username: "admin",
		Password: "rethink",
		MaxIdle:  10,
		MaxOpen:  2000,
	}

	err := generic.Initialize(t.Context(), log, connectOpts, generic.AsnPoolRange(uint(1), uint(100)), generic.VrfPoolRange(uint(1), uint(100)))
	assert.NoError(t, err)
	if err != nil {
		reader, err := rethinkdbContainer.Logs(t.Context())
		require.NoError(t, err)
		logs, err := io.ReadAll(reader)
		require.NoError(t, err)
		t.Log(string(logs))
	}

	ds, err := generic.New(log, connectOpts)
	require.NoError(t, err)

	return ds, connectOpts, closer
}

func databaseNameFromT(t testing.TB) string {
	return strings.ReplaceAll(t.Name(), "/", "-")
}
