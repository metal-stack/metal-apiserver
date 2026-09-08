package pg_test

import (
	"database/sql"
	"errors"
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

// UserProfile is our nested sample entity struct
type Address struct {
	City    string `json:"city"`
	Country string `json:"country"`
}

type UserProfile struct {
	Name    string  `json:"name"`
	Age     int     `json:"age"`
	Address Address `json:"address"`
}

func TestGenericRepository(t *testing.T) {
	ctx := t.Context()
	// Connect to test PostgreSQL database
	postgres, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithPassword("password"),
		postgres.BasicWaitStrategies(),
		testcontainers.WithTmpfs(map[string]string{"/var/lib/postgresql": "rw"}),
	)
	require.NoError(t, err)

	connectionString, err := postgres.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("postgres", connectionString)
	require.NoError(t, err)

	// Setup table schema
	setupQuery := pg.DDL
	_, err = db.ExecContext(ctx, setupQuery)
	require.NoError(t, err)

	repo := pg.NewGenericRepository[UserProfile](slog.Default(), db)
	userID := uuid.NewV7()

	// 1. Test Create (Insert)
	initialProfile := UserProfile{
		Name: "Alice",
		Age:  30,
		Address: Address{
			City:    "Munich",
			Country: "Germany",
		},
	}

	err = repo.Create(ctx, userID, initialProfile)
	require.NoError(t, err)

	// Fetch to verify insertion and version
	ent, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, ent)
	require.Equal(t, 1, ent.Version)
	require.Equal(t, "Munich", ent.Data.Address.City)

	// 2. Test Optimistic Locking (Success Path)
	updatedProfile := ent.Data
	updatedProfile.Address.City = "Berlin"

	err = repo.Update(ctx, userID, ent.Version, updatedProfile)
	require.NoError(t, err)

	// Fetch to verify version bump
	entUpdated, _ := repo.Get(ctx, userID)
	require.Equal(t, 2, entUpdated.Version)

	// 3. Test Optimistic Locking (Failure Path - Stale Version)
	staleProfile := updatedProfile
	staleProfile.Name = "Alice Stale"

	// Passing stale version 1 instead of current version 2
	err = repo.Update(ctx, userID, 1, staleProfile)
	if !errors.Is(err, pg.ErrOptimisticLockConflict) {
		t.Errorf("expected ErrOptimisticLockConflict, got: %v", err)
	}

	// Updating a non-existent id returns ErrNotFound, not a lock conflict
	err = repo.Update(ctx, uuid.NewV7(), 1, updatedProfile)
	require.ErrorIs(t, err, pg.ErrNotFound)

	// 4. Test Query Interface (Nested JSON Path)
	// Insert a second entity to test filtering
	user2ID := uuid.NewV7()
	err = repo.Create(ctx, user2ID, UserProfile{
		Name: "Bob",
		Age:  25,
		Address: Address{
			City:    "Vienna",
			Country: "Austria",
		},
	})
	require.NoError(t, err)

	// Query by nested JSON path: address.city = 'Berlin'
	results, err := repo.Query(ctx, []pg.QueryFilter{
		{Path: "address.city", Op: "=", Value: "Berlin"},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, userID, results[0].ID)

	// Query by nested JSON path with the PathOf helper
	countryPath := pg.PathOf(func(u *UserProfile) any {
		return &u.Address.Country
	})
	results2, err := repo.Query(ctx, []pg.QueryFilter{
		{Path: countryPath, Op: "=", Value: "Austria"},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results2, 1)
	require.Equal(t, user2ID, results2[0].ID)

	// 5. Test Delete
	err = repo.Delete(ctx, userID)
	require.NoError(t, err)

	// Get after delete returns ErrNotFound
	ent, err = repo.Get(ctx, userID)
	require.ErrorIs(t, err, pg.ErrNotFound)
	require.Nil(t, ent)

	// Deleting a non-existent entity returns ErrNotFound
	err = repo.Delete(ctx, userID)
	require.ErrorIs(t, err, pg.ErrNotFound)
}

func TestGenericRepositoryPagination(t *testing.T) {
	ctx := t.Context()
	postgres, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithPassword("password"),
		postgres.BasicWaitStrategies(),
		testcontainers.WithTmpfs(map[string]string{"/var/lib/postgresql": "rw"}),
	)
	require.NoError(t, err)

	connectionString, err := postgres.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("postgres", connectionString)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, pg.DDL)
	require.NoError(t, err)

	repo := pg.NewGenericRepository[UserProfile](slog.Default(), db)

	// Insert 5 entities sharing a common city so paging over the result is meaningful
	const total = 5
	for i := range total {
		err = repo.Create(ctx, uuid.NewV7(), UserProfile{
			Name:    "User" + strconv.Itoa(i),
			Age:     i,
			Address: Address{City: "Munich", Country: "Germany"},
		})
		require.NoError(t, err)
	}

	filter := []pg.QueryFilter{{Path: "address.city", Op: "=", Value: "Munich"}}

	// No pagination returns everything
	all, err := repo.Query(ctx, filter, nil)
	require.NoError(t, err)
	require.Len(t, all, total)

	// Limit alone
	limited, err := repo.Query(ctx, filter, &pg.Pagination{Limit: 2})
	require.NoError(t, err)
	require.Len(t, limited, 2)

	// Offset alone skips the first N
	offset, err := repo.Query(ctx, filter, &pg.Pagination{Offset: 2})
	require.NoError(t, err)
	require.Len(t, offset, total-2)

	// Limit + Offset pages through the full set without overlap
	var paged []pg.Entity[UserProfile]
	for page := 0; page < total; page += 2 {
		chunk, err := repo.Query(ctx, filter, &pg.Pagination{Limit: 2, Offset: page})
		require.NoError(t, err)
		paged = append(paged, chunk...)
	}
	require.Len(t, paged, total)
}
