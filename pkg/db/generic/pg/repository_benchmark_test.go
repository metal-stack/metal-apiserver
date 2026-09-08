package pg_test

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"testing"

	"uuid"

	_ "github.com/lib/pq"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Shallow entity (1-level)
type SimpleEntity struct {
	Name string `json:"name"`
	City string `json:"city"`
}

// Deeply nested entity (5-levels)
type Level4 struct {
	Value string `json:"value"`
}
type Level3 struct {
	Level4 Level4 `json:"level4"`
}
type Level2 struct {
	Level3 Level3 `json:"level3"`
}
type Level1 struct {
	Level2 Level2 `json:"level2"`
}
type DeepEntity struct {
	Root Level1 `json:"root"`
}

func setupBenchmarkDB(b *testing.B) *sql.DB {
	b.Helper()
	ctx := b.Context()

	postgres, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithPassword("password"),
		postgres.BasicWaitStrategies(),
		testcontainers.WithTmpfs(map[string]string{"/var/lib/postgresql": "rw"}),
	)
	require.NoError(b, err)

	connectionString, err := postgres.ConnectionString(ctx, "sslmode=disable")
	require.NoError(b, err)

	db, err := sql.Open("postgres", connectionString)
	require.NoError(b, err)

	setupQuery := `
		CREATE TABLE IF NOT EXISTS generic_entities (
			id UUID PRIMARY KEY,
			entity_type TEXT NOT NULL,
			version INT NOT NULL DEFAULT 1,
			data JSONB NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_generic_entities_type ON generic_entities(entity_type);
		CREATE INDEX IF NOT EXISTS idx_generic_entities_data ON generic_entities USING gin (data);
	`
	_, err = db.Exec(setupQuery)
	require.NoError(b, err)

	return db
}

// Benchmark Query Performance: Shallow vs Deep Nested Paths
func BenchmarkQueryPerformance(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer func() {
		_ = db.Close()
	}()
	ctx := b.Context()

	log := slog.Default()
	simpleRepo := pg.NewGenericRepository[SimpleEntity](log, db)
	deepRepo := pg.NewGenericRepository[DeepEntity](log, db)

	// Seed 1,000 records each to test index search overhead
	for i := range 1000 {
		id := uuid.NewV7()
		valStr := strconv.Itoa(i)

		_ = simpleRepo.Create(ctx, id, SimpleEntity{
			Name: "User " + valStr,
			City: "City_" + valStr,
		})

		_ = deepRepo.Create(ctx, id, DeepEntity{
			Root: Level1{
				Level2: Level2{
					Level3: Level3{
						Level4: Level4{
							Value: "DeepVal_" + valStr,
						},
					},
				},
			},
		})
	}

	b.ResetTimer()

	b.Run("Shallow_Query_Path_1Level", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			target := "City_" + strconv.Itoa(i%1000)
			_, err := simpleRepo.Query(ctx, []pg.QueryFilter{
				{Path: "city", Op: "=", Value: target},
			}, nil)
			require.NoError(b, err)
		}
	})

	b.Run("Deep_Query_Path_5Levels", func(b *testing.B) {
		queryPath := pg.SelectorPath[DeepEntity]("Root", "Level2", "Level3", "Level4", "Value")
		require.Equal(b, "root.level2.level3.level4.value", queryPath)
		for i := 0; i < b.N; i++ {
			target := "DeepVal_" + strconv.Itoa(i%1000)
			_, err := deepRepo.Query(ctx, []pg.QueryFilter{
				{Path: queryPath, Op: "=", Value: target},
			}, nil)
			require.NoError(b, err)
		}
	})
}

// Benchmark Update Performance with Optimistic Locking
func BenchmarkUpdatePerformance(b *testing.B) {
	db := setupBenchmarkDB(b)
	defer func() {
		_ = db.Close()
	}()
	ctx := b.Context()

	log := slog.Default()
	simpleRepo := pg.NewGenericRepository[SimpleEntity](log, db)
	deepRepo := pg.NewGenericRepository[DeepEntity](log, db)

	simpleID := uuid.NewV7()
	deepID := uuid.NewV7()

	_ = simpleRepo.Create(ctx, simpleID, SimpleEntity{Name: "Initial", City: "Munich"})
	_ = deepRepo.Create(ctx, deepID, DeepEntity{
		Root: Level1{Level2: Level2{Level3: Level3{Level4: Level4{Value: "Initial"}}}},
	})

	b.ResetTimer()

	b.Run("Shallow_Update", func(b *testing.B) {
		version := int32(1)

		for b.Loop() {
			err := simpleRepo.Update(ctx, simpleID, version, SimpleEntity{
				Name: "Updated",
				City: fmt.Sprintf("City_%d", version),
			})
			require.NoError(b, err)
			version++
		}
	})

	b.Run("Deep_Update", func(b *testing.B) {
		version := int32(1)
		for b.Loop() {
			err := deepRepo.Update(ctx, deepID, version, DeepEntity{
				Root: Level1{Level2: Level2{Level3: Level3{Level4: Level4{Value: fmt.Sprintf("Val_%d", version)}}}},
			})
			require.NoError(b, err)
			version++
		}
	})
}
