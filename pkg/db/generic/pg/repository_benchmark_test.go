package pg_test

import (
	"fmt"
	"log/slog"
	"strconv"
	"testing"

	"uuid"

	_ "github.com/lib/pq"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
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

// Benchmark Query Performance: Shallow vs Deep Nested Paths
func BenchmarkQueryPerformance(b *testing.B) {
	ctx := b.Context()
	log := slog.Default()
	db, closer := test.StartPostgres(b, log)
	defer closer()

	simpleRepo, err := pg.NewGenericRepository[SimpleEntity](log, db)
	require.NoError(b, err)
	deepRepo, err := pg.NewGenericRepository[DeepEntity](log, db)
	require.NoError(b, err)

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
	ctx := b.Context()
	log := slog.Default()
	db, closer := test.StartPostgres(b, log)
	defer closer()

	simpleRepo, err := pg.NewGenericRepository[SimpleEntity](log, db)
	require.NoError(b, err)
	deepRepo, err := pg.NewGenericRepository[DeepEntity](log, db)
	require.NoError(b, err)

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
