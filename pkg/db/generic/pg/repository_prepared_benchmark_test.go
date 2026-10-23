package pg_test

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"testing"

	"uuid"

	_ "github.com/lib/pq"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

// This benchmark isolates the effect of preparing SQL statements on the wire
// protocol. Every sub-benchmark runs the exact query used by GenericRepository
// (see repository.go) twice:
//
//   - _Direct:   db.ExecContext / db.QueryRowContext with the inline query
//     string, i.e. the current implementation. lib/pq sends Parse+Describe+
//     Bind+Execute for every single call.
//   - _Prepared: db.Prepare once, then stmt.Exec / stmt.QueryRow. lib/pq
//     sends Parse+Describe once per connection and only Bind+Execute per
//     call afterwards.
//
// Both arms share the same table, the same seed data and the same connection
// pool. Every arm warms up (buffer cache, lazy per-connection Parse, CPU
// frequency) before the timer starts, so the delta is the steady-state cost
// of the preparation strategy alone.

const (
	benchTable  = `bench_prepared_entity`
	warmupIters = 256
)

type benchRow struct {
	ID      string
	Version int32
	Data    []byte
}

func setupBenchTable(b *testing.B, db *sql.DB) string {
	b.Helper()
	ctx := b.Context()

	_, err := db.ExecContext(ctx, fmt.Sprintf(`
		DROP TABLE IF EXISTS %s;
		CREATE TABLE %s (
			id TEXT PRIMARY KEY,
			version INT NOT NULL DEFAULT 1,
			data JSONB NOT NULL
		);
		CREATE INDEX idx_%s_data ON %s USING gin (data);
	`, benchTable, benchTable, benchTable, benchTable))
	require.NoError(b, err)

	pointID := uuid.NewV7().String()
	payload := `{"name":"point","city":"Munich","extra":"xxyyzz"}`
	_, err = db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s (id, version, data) VALUES ($1, 1, $2)`, benchTable), pointID, payload)
	require.NoError(b, err)

	for i := range 1000 {
		id := uuid.NewV7().String()
		p := fmt.Sprintf(`{"name":"User %d","city":"City_%d"}`, i, i)
		_, err := db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s (id, version, data) VALUES ($1, 1, $2)`, benchTable), id, p)
		require.NoError(b, err)
	}

	return pointID
}

var (
	qGet    = fmt.Sprintf("SELECT id, version, data FROM %s WHERE id = $1", benchTable)
	qCreate = fmt.Sprintf("INSERT INTO %s (id, version, data) VALUES ($1, 1, $2) ON CONFLICT (id) DO NOTHING", benchTable)
	qUpsert = fmt.Sprintf(`INSERT INTO %s (id, version, data) VALUES ($1, 1, $2)
		ON CONFLICT (id) DO UPDATE
		SET data = EXCLUDED.data,
		    version = CASE WHEN %s.data IS DISTINCT FROM EXCLUDED.data
		                  THEN %s.version + 1
		                  ELSE %s.version END`, benchTable, benchTable, benchTable, benchTable)
	qUpdate = fmt.Sprintf("UPDATE %s SET data = $1, version = version + 1 WHERE id = $2 AND version = $3", benchTable)
	qDelete = fmt.Sprintf("DELETE FROM %s WHERE id = $1", benchTable)
	qExists = fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE id = $1)", benchTable)
	qQuery  = fmt.Sprintf("SELECT id, version, data FROM %s WHERE data @> $1 ORDER BY id", benchTable)
)

// currentVersion reads the live version of the point row so the Update arms
// always take the rows-affected=1 path, regardless of what earlier
// sub-benchmarks did to the row.
func currentVersion(b *testing.B, db *sql.DB, ctx context.Context, id string) int32 {
	b.Helper()
	var v int32
	err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT version FROM %s WHERE id = $1", benchTable), id).Scan(&v)
	require.NoError(b, err)
	return v
}

func BenchmarkStatementPreparation(b *testing.B) {
	log := slog.Default()
	db, closer := test.StartPostgres(b, log)
	defer closer()

	pointID := setupBenchTable(b, db)
	ctx := b.Context()
	payload := `{"name":"updated","city":"Berlin"}`

	b.Run("Get_Direct", func(b *testing.B) {
		var row benchRow
		f := func() {
			err := db.QueryRowContext(ctx, qGet, pointID).Scan(&row.ID, &row.Version, &row.Data)
			require.NoError(b, err)
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})

	b.Run("Get_Prepared", func(b *testing.B) {
		stmt, err := db.Prepare(qGet)
		require.NoError(b, err)
		defer func() {
			_ = stmt.Close()
		}()
		var row benchRow
		f := func() {
			err := stmt.QueryRowContext(ctx, pointID).Scan(&row.ID, &row.Version, &row.Data)
			require.NoError(b, err)
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})

	b.Run("Get_Parallel_Direct", func(b *testing.B) {
		var row benchRow
		f := func() {
			err := db.QueryRowContext(ctx, qGet, pointID).Scan(&row.ID, &row.Version, &row.Data)
			require.NoError(b, err)
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var row benchRow
			g := func() {
				err := db.QueryRowContext(ctx, qGet, pointID).Scan(&row.ID, &row.Version, &row.Data)
				require.NoError(b, err)
			}
			for pb.Next() {
				g()
			}
		})
	})

	b.Run("Get_Parallel_Prepared", func(b *testing.B) {
		stmt, err := db.Prepare(qGet)
		require.NoError(b, err)
		defer func() {
			_ = stmt.Close()
		}()
		var row benchRow
		f := func() {
			err := stmt.QueryRowContext(ctx, pointID).Scan(&row.ID, &row.Version, &row.Data)
			require.NoError(b, err)
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var row benchRow
			g := func() {
				err := stmt.QueryRowContext(ctx, pointID).Scan(&row.ID, &row.Version, &row.Data)
				require.NoError(b, err)
			}
			for pb.Next() {
				g()
			}
		})
	})

	b.Run("Create_Direct", func(b *testing.B) {
		f := func() {
			_, err := db.ExecContext(ctx, qCreate, pointID, payload)
			require.NoError(b, err)
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})

	b.Run("Create_Prepared", func(b *testing.B) {
		stmt, err := db.Prepare(qCreate)
		require.NoError(b, err)
		defer func() {
			_ = stmt.Close()
		}()
		f := func() {
			_, err := stmt.ExecContext(ctx, pointID, payload)
			require.NoError(b, err)
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})

	b.Run("Upsert_Direct", func(b *testing.B) {
		f := func() {
			_, err := db.ExecContext(ctx, qUpsert, pointID, payload)
			require.NoError(b, err)
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})

	b.Run("Upsert_Prepared", func(b *testing.B) {
		stmt, err := db.Prepare(qUpsert)
		require.NoError(b, err)
		defer func() {
			_ = stmt.Close()
		}()
		f := func() {
			_, err := stmt.ExecContext(ctx, pointID, payload)
			require.NoError(b, err)
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})

	b.Run("Update_Direct", func(b *testing.B) {
		version := currentVersion(b, db, ctx, pointID)
		f := func() {
			res, err := db.ExecContext(ctx, qUpdate, payload, pointID, version)
			require.NoError(b, err)
			n, err := res.RowsAffected()
			require.NoError(b, err)
			if n == 1 {
				version++
			}
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})

	b.Run("Update_Prepared", func(b *testing.B) {
		stmt, err := db.Prepare(qUpdate)
		require.NoError(b, err)
		defer func() {
			_ = stmt.Close()
		}()
		version := currentVersion(b, db, ctx, pointID)
		f := func() {
			res, err := stmt.ExecContext(ctx, payload, pointID, version)
			require.NoError(b, err)
			n, err := res.RowsAffected()
			require.NoError(b, err)
			if n == 1 {
				version++
			}
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})

	b.Run("Delete_Direct", func(b *testing.B) {
		// Recreate the point row: with -count > 1 earlier counts already
		// deleted it, and every count must measure the matching-row path.
		_, err := db.ExecContext(ctx, qCreate, pointID, payload)
		require.NoError(b, err)
		f := func() {
			_, err := db.ExecContext(ctx, qDelete, pointID)
			require.NoError(b, err)
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})

	b.Run("Delete_Prepared", func(b *testing.B) {
		stmt, err := db.Prepare(qDelete)
		require.NoError(b, err)
		defer func() {
			_ = stmt.Close()
		}()
		_, err = db.ExecContext(ctx, qCreate, pointID, payload)
		require.NoError(b, err)
		f := func() {
			_, err := stmt.ExecContext(ctx, pointID)
			require.NoError(b, err)
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})

	b.Run("Exists_Direct", func(b *testing.B) {
		var found bool
		f := func() {
			err := db.QueryRowContext(ctx, qExists, pointID).Scan(&found)
			require.NoError(b, err)
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})

	b.Run("Exists_Prepared", func(b *testing.B) {
		stmt, err := db.Prepare(qExists)
		require.NoError(b, err)
		defer func() {
			_ = stmt.Close()
		}()
		var found bool
		f := func() {
			err := stmt.QueryRowContext(ctx, pointID).Scan(&found)
			require.NoError(b, err)
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})

	b.Run("FilterQuery_Direct", func(b *testing.B) {
		target := `{"city":"City_42"}`
		f := func() {
			rows, err := db.QueryContext(ctx, qQuery, target)
			require.NoError(b, err)
			_ = rows.Close()
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})

	b.Run("FilterQuery_Prepared", func(b *testing.B) {
		stmt, err := db.Prepare(qQuery)
		require.NoError(b, err)
		defer func() {
			_ = stmt.Close()
		}()
		target := `{"city":"City_42"}`
		f := func() {
			rows, err := stmt.QueryContext(ctx, target)
			require.NoError(b, err)
			_ = rows.Close()
		}
		for range warmupIters {
			f()
		}
		b.ResetTimer()
		for b.Loop() {
			f()
		}
	})
}
