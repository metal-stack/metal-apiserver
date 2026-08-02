package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Initialize(ctx context.Context, log *slog.Logger, pool *pgxpool.Pool) error {
	log = log.WithGroup("datastore")
	log.Info("initializing postgres database tables")

	ds, err := New(log, Config{Pool: pool})
	if err != nil {
		return fmt.Errorf("unable to create datastore: %w", err)
	}

	dsi := ds.(*datastore)

	for _, tableName := range dsi.tableNames {
		if err := createTable(ctx, pool, tableName); err != nil {
			return fmt.Errorf("cannot create %s table: %w", tableName, err)
		}
	}

	// Create integer pool tables
	for _, poolTable := range []string{"asnpool", "vrfpool"} {
		if err := createPoolTable(ctx, pool, poolTable); err != nil {
			return fmt.Errorf("cannot create pool table %s: %w", poolTable, err)
		}
	}

	log.Info("database init complete")

	return nil
}

func createTable(ctx context.Context, pool *pgxpool.Pool, tableName string) error {
	_, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			data JSONB NOT NULL,
			created TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			changed TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			generation BIGINT NOT NULL DEFAULT 0
		)
	`, quoteIdent(tableName)))
	return err
}

func createPoolTable(ctx context.Context, pool *pgxpool.Pool, tableName string) error {
	_, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id BIGINT PRIMARY KEY
		)
	`, quoteIdent(tableName)))
	return err
}

func InitializeIntegerPool(ctx context.Context, pool *pgxpool.Pool, tableName string, min, max uint) error {
	var count int64
	err := pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT count(*) FROM %s`, quoteIdent(tableName)),
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("cannot check pool %s: %w", tableName, err)
	}
	if count > 0 {
		return nil
	}

	// Insert all integers in the range
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	batch := &pgx.Batch{}
	for i := min; i <= max; i++ {
		batch.Queue(fmt.Sprintf(`INSERT INTO %s (id) VALUES ($1) ON CONFLICT DO NOTHING`, quoteIdent(tableName)), i)
	}

	br := conn.SendBatch(ctx, batch)
	defer br.Close()

	for range max - min + 1 {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("cannot initialize pool %s: %w", tableName, err)
		}
	}

	return nil
}

func isPgUniqueViolation(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "23505"))
}
