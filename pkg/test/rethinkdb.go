package test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"uuid"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tlog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

const (
	rethinkDbImage = "rethinkdb:2.4.4-bookworm-slim"

	// rethinkDbContainerName is a fixed name so that the single rethinkdb
	// container is reused by all test binaries of a test run instead of a new
	// container being started for every package.
	rethinkDbContainerName = "metal-apiserver-rethinkdb"
)

// init makes all test binaries of a single `go test` run share one
// testcontainers session. Every test binary is spawned by the same go process,
// so deriving the session id from its pid makes testcontainers use a single
// reaper for the entire run. This is required so that the shared rethinkdb
// container is reused by every package and is not terminated by the reaper as
// soon as the first package's tests are done.
func init() {
	if os.Getenv("TESTCONTAINERS_SESSION_ID") != "" {
		return
	}

	_ = os.Setenv("TESTCONTAINERS_SESSION_ID", fmt.Sprintf("metal-apiserver-%d", os.Getppid()))
}

var (
	rethinkDbEndpoint string
	rethinkDbMtx      sync.Mutex
)

// StartRethink returns a datastore backed by a rethinkdb instance that is
// shared by all tests of a test run. Every test gets its own database inside
// this single instance, named after the package and the test, so tests can
// safely run in parallel.
func StartRethink(t testing.TB, log *slog.Logger) (generic.Datastore, r.ConnectOpts, func()) {
	connectOpts := r.ConnectOpts{
		Address:    sharedRethinkDbEndpoint(t),
		Database:   databaseNameFromT(t),
		Username:   "admin",
		Password:   "rethink",
		InitialCap: 20,
		MaxOpen:    2000,
	}

	err := generic.Initialize(t.Context(), log, connectOpts, generic.AsnPoolRange(uint(1), uint(100)), generic.VrfPoolRange(uint(1), uint(100)), generic.NewMutexOptCheckInterval(3*time.Second))
	require.NoError(t, err)

	ds, err := generic.New(log, connectOpts)
	require.NoError(t, err)

	closer := func() {
		// The rethinkdb container is shared by all tests and must not be
		// terminated here. It is cleaned up by testcontainers once the test
		// run is done. The database created for this test is intentionally
		// left behind so that parallel tests do not interfere with each other.
	}

	return ds, connectOpts, closer
}

// sharedRethinkDbEndpoint lazily starts the single rethinkdb container and
// returns its endpoint. The container is started only once, even when tests
// call this concurrently, but the potentially expensive per-test database
// initialization is performed outside of the lock to allow tests to run in
// parallel.
func sharedRethinkDbEndpoint(t testing.TB) string {
	rethinkDbMtx.Lock()
	defer rethinkDbMtx.Unlock()

	if rethinkDbEndpoint != "" {
		return rethinkDbEndpoint
	}

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
		// a fixed name plus reuse ensures only a single rethinkdb instance
		// lives for all packages of a test run
		testcontainers.WithReuseByName(rethinkDbContainerName),
	)
	require.NoError(t, err)

	rethinkDbEndpoint, err = c.PortEndpoint(ctx, "28015/tcp", "")
	require.NoError(t, err)

	return rethinkDbEndpoint
}

func databaseNameFromT(t testing.TB) string {
	pkg := callerPackagePath()
	if pkg == "" {
		return sanitizeDBName(t.Name())
	}

	// Prefixing the test name with the package of the caller keeps database
	// names unique across packages, even if two packages contain a test with
	// the same name. RethinkDB only allows A-Z, a-z, 0-9, _ and - in database
	// names, hence everything else (slashes of subtests, dots of import paths,
	// ...) is replaced.
	return sanitizeDBName(pkg + "-" + t.Name())
}

func sanitizeDBName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		default:
			return '-'
		}
	}, s)
}

// testPackagePath is the import path of this helper package. Frames belonging
// to it are skipped when determining the caller of StartRethink.
const testPackagePath = "github.com/metal-stack/metal-apiserver/pkg/test"

// callerPackagePath returns the import path of the first function outside of
// this helper package that is on the call stack. This is the package of the
// test that eventually called StartRethink.
func callerPackagePath() string {
	pcs := make([]uintptr, 32)

	// skip runtime.Callers and callerPackagePath itself, databaseNameFromT and
	// StartRethink belong to testPackagePath and are skipped below anyway
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])

	for {
		frame, more := frames.Next()

		pkg := packageFromFuncName(frame.Function)
		if pkg != "" && pkg != testPackagePath {
			return pkg
		}

		if !more {
			return ""
		}
	}
}

// packageFromFuncName extracts the import path from a fully qualified function
// name such as "github.com/foo/bar/pkg.TestSomething" or
// "github.com/foo/bar/pkg.(*T).Method".
func packageFromFuncName(fn string) string {
	slash := strings.LastIndex(fn, "/")
	dot := strings.Index(fn[slash+1:], ".")
	if dot < 0 {
		return ""
	}

	return fn[:slash+1+dot]
}

func containerName(t testing.TB) string {
	containerUUID := uuid.New().String()
	suffix, _, _ := strings.Cut(containerUUID, "-")
	return strings.ReplaceAll(t.Name(), "/", "-") + "-" + suffix
}
