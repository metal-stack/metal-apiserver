package test

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tlog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

const rethinkDbImage = "rethinkdb:2.4.4-bookworm-slim"

var (
	connectOpts r.ConnectOpts
	endpoint    string
	closer      func()
	mtx         sync.Mutex
	count       int
)

func StartRethink(t testing.TB, log *slog.Logger) (generic.Datastore, r.ConnectOpts, func()) {
	mtx.Lock()
	defer mtx.Unlock()

	if endpoint == "" {
		ctx := context.Background()

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

		endpoint, err = c.PortEndpoint(ctx, "28015/tcp", "")
		require.NoError(t, err)

		closer = func() {
			// TODO: clean up database of this test

			// we do not terminate the container here because it's very complex with a shared ds
			// testcontainers will cleanup the database by itself
		}
	}

	connectOpts = r.ConnectOpts{
		Address:  endpoint,
		Database: databaseNameFromT(t),
		Username: "admin",
		Password: "rethink",
		MaxIdle:  10,
		MaxOpen:  2000,
	}

	err := generic.Initialize(t.Context(), log, connectOpts, generic.AsnPoolRange(uint(1), uint(100)), generic.VrfPoolRange(uint(1), uint(100)))
	require.NoError(t, err)

	ds, err := generic.New(log, connectOpts)
	require.NoError(t, err)

	return ds, connectOpts, closer

}

func databaseNameFromT(t testing.TB) string {
	return strings.ReplaceAll(t.Name(), "/", "-")
}
