package test

import (
	"database/sql"
	"log/slog"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func StartPostgres(t testing.TB, log *slog.Logger) (*sql.DB, func()) {
	t.Helper()
	ctx := t.Context()

	pgContainer, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithDatabase("benchdb"),
		postgres.WithUsername("benchuser"),
		postgres.WithPassword("benchpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
		testcontainers.WithName(containerName(t)),
	)
	require.NoError(t, err)

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)

	// Tune connection pool for benchmark concurrency
	maxConns := runtime.GOMAXPROCS(0) * 4
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)

	cleanup := func() {
		_ = db.Close()
		_ = pgContainer.Terminate(ctx)
	}

	return db, cleanup
}
