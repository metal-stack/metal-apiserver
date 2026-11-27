package pg_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"
	"uuid"

	"github.com/lib/pq"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
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

type Device struct {
	Model string `json:"model"`
}

func TestGenericRepository(t *testing.T) {
	ctx := t.Context()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[UserProfile](log, db)
	require.NoError(t, err)
	userID := uuid.NewV7().String()

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
	require.Equal(t, int32(1), ent.Version)
	require.Equal(t, "Munich", ent.Data.Address.City)

	// Creating an existing id fails with ErrAlreadyExists and leaves the row untouched
	err = repo.Create(ctx, userID, UserProfile{Name: "Impostor"})
	require.ErrorIs(t, err, pg.ErrAlreadyExists)
	ent, err = repo.Get(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, int32(1), ent.Version)
	require.Equal(t, "Alice", ent.Data.Name)

	// 2. Test Optimistic Locking (Success Path)
	updatedProfile := ent.Data
	updatedProfile.Address.City = "Berlin"

	err = repo.Update(ctx, userID, ent.Version, updatedProfile)
	require.NoError(t, err)

	// Fetch to verify version bump
	entUpdated, _ := repo.Get(ctx, userID)
	require.Equal(t, int32(2), entUpdated.Version)

	// 3. Test Optimistic Locking (Failure Path - Stale Version)
	staleProfile := updatedProfile
	staleProfile.Name = "Alice Stale"

	// Passing stale version 1 instead of current version 2
	err = repo.Update(ctx, userID, 1, staleProfile)
	if !errors.Is(err, pg.ErrOptimisticLockConflict) {
		t.Errorf("expected ErrOptimisticLockConflict, got: %v", err)
	}

	// Updating a non-existent id returns ErrNotFound, not a lock conflict
	err = repo.Update(ctx, uuid.NewV7().String(), 1, updatedProfile)
	require.ErrorIs(t, err, pg.ErrNotFound)

	// 4. Test Query Interface (Nested JSON Path)
	// Insert a second entity to test filtering
	user2ID := uuid.NewV7().String()
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

	// Query by nested JSON path with the SelectorPath helper
	countryPath := pg.SelectorPath[UserProfile]("Address", "Country")
	results2, err := repo.Query(ctx, []pg.QueryFilter{
		{Path: countryPath, Op: "=", Value: "Austria"},
	}, nil)
	require.NoError(t, err)
	require.Len(t, results2, 1)
	require.Equal(t, user2ID, results2[0].ID)

	// Query all
	results3, err := repo.Query(ctx, nil, nil)
	require.NoError(t, err)
	require.Len(t, results3, 2)
	var data3 []UserProfile
	for _, e := range results3 {
		data3 = append(data3, e.Data)
	}
	require.ElementsMatch(t, data3, []UserProfile{
		{
			Name: "Bob",
			Age:  25,
			Address: Address{
				City:    "Vienna",
				Country: "Austria",
			},
		},
		{
			Name: "Alice",
			Age:  30,
			Address: Address{
				City:    "Berlin",
				Country: "Germany",
			},
		},
	})

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

func TestGenericRepositoryWatch(t *testing.T) {
	ctx := t.Context()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, dsn, closer := test.StartPostgresWithDSN(t, log)
	defer closer()

	watcher, err := pg.NewWatcher(log, db, dsn)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, watcher.Close())
	}()

	repo, err := pg.NewGenericRepository[UserProfile](log, db, pg.WithWatcher(watcher))
	require.NoError(t, err)

	userID := uuid.NewV7().String()

	watchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	changes, err := repo.Watch(watchCtx, userID)
	require.NoError(t, err)

	profile := UserProfile{
		Name:    "Alice",
		Age:     30,
		Address: Address{City: "Munich", Country: "Germany"},
	}
	require.NoError(t, repo.Create(ctx, userID, profile))

	insert := receiveChange(t, changes)
	require.Equal(t, UserProfile{}, insert.Old)
	require.Equal(t, "Alice", insert.New.Name)
	require.Equal(t, "Munich", insert.New.Address.City)

	ent, err := repo.Get(ctx, userID)
	require.NoError(t, err)

	updated := ent.Data
	updated.Address.City = "Berlin"
	require.NoError(t, repo.Update(ctx, userID, ent.Version, updated))

	update := receiveChange(t, changes)
	require.Equal(t, "Munich", update.Old.Address.City)
	require.Equal(t, "Berlin", update.New.Address.City)

	require.NoError(t, repo.Delete(ctx, userID))

	deletion := receiveChange(t, changes)
	require.Equal(t, "Berlin", deletion.Old.Address.City)
	require.Equal(t, UserProfile{}, deletion.New)
}

func receiveChange(t *testing.T, changes <-chan struct {
	Old UserProfile
	New UserProfile
}) struct {
	Old UserProfile
	New UserProfile
} {
	t.Helper()

	select {
	case change, ok := <-changes:
		require.True(t, ok, "watch channel was closed unexpectedly")
		return change
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for a change")
		return struct {
			Old UserProfile
			New UserProfile
		}{}
	}
}

// TestEntityTableNotifyChange verifies that the single shared trigger
// function serves every per-entity table and derives the entity type from
// the table it fired on, so two tables may hold the same id independently.
func TestEntityTableNotifyChange(t *testing.T) {
	ctx := t.Context()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, dsn, closer := test.StartPostgresWithDSN(t, log)
	defer closer()

	listener := pq.NewListener(dsn, 10*time.Second, time.Minute, nil)
	require.NoError(t, listener.Listen(pg.NotifyChannel))
	defer func() {
		require.NoError(t, listener.Close())
	}()

	watcher, err := pg.NewWatcher(log, db, dsn)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, watcher.Close())
	}()

	profileRepo, err := pg.NewGenericRepository[UserProfile](log, db, pg.WithWatcher(watcher))
	require.NoError(t, err)
	deviceRepo, err := pg.NewGenericRepository[Device](log, db, pg.WithWatcher(watcher))
	require.NoError(t, err)

	// The entity type is the table name as Postgres stores it: unquoted
	// identifiers are folded to lowercase.
	const (
		userProfileTable = "userprofile"
		deviceTable      = "device"
	)

	id := uuid.NewV7().String()

	require.NoError(t, profileRepo.Create(ctx, id, UserProfile{Name: "Alice"}))
	assertNotificationPayload(t, receiveNotification(t, listener), id, userProfileTable, "INSERT")

	require.NoError(t, deviceRepo.Create(ctx, id, Device{Model: "M1"}))
	assertNotificationPayload(t, receiveNotification(t, listener), id, deviceTable, "INSERT")

	ent, err := profileRepo.Get(ctx, id)
	require.NoError(t, err)
	updated := ent.Data
	updated.Name = "Bob"
	require.NoError(t, profileRepo.Update(ctx, id, ent.Version, updated))
	assertNotificationPayload(t, receiveNotification(t, listener), id, userProfileTable, "UPDATE")

	require.NoError(t, deviceRepo.Delete(ctx, id))
	assertNotificationPayload(t, receiveNotification(t, listener), id, deviceTable, "DELETE")

	// Constructing a repository again does not duplicate the trigger, every
	// change is announced exactly once.
	_, err = pg.NewGenericRepository[Device](log, db, pg.WithWatcher(watcher))
	require.NoError(t, err)
	require.NoError(t, deviceRepo.Create(ctx, id, Device{Model: "M2"}))
	assertNotificationPayload(t, receiveNotification(t, listener), id, deviceTable, "INSERT")

	// A repository without a watcher leaves an existing trigger in place.
	_, err = pg.NewGenericRepository[Device](log, db)
	require.NoError(t, err)
	require.NoError(t, deviceRepo.Delete(ctx, id))
	assertNotificationPayload(t, receiveNotification(t, listener), id, deviceTable, "DELETE")

	// A table of a repository without a watcher gets no trigger, writes to it
	// are not announced.
	unwatched, err := pg.NewGenericRepository[Unwatched](log, db)
	require.NoError(t, err)
	require.NoError(t, unwatched.Create(ctx, id, Unwatched{Name: "x"}))

	select {
	case n := <-listener.NotificationChannel():
		t.Fatalf("unexpected notification for a table without watcher: %v", n)
	case <-time.After(500 * time.Millisecond):
	}
}

type Unwatched struct {
	Name string `json:"name"`
}

func receiveNotification(t *testing.T, l *pq.Listener) pq.Notification {
	t.Helper()

	select {
	case n := <-l.NotificationChannel():
		require.NotNil(t, n)
		return *n
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for a notification")
		return pq.Notification{}
	}
}

func assertNotificationPayload(t *testing.T, n pq.Notification, id, entityType, op string) {
	t.Helper()

	var payload struct {
		ID         string `json:"id"`
		EntityType string `json:"entity_type"`
		Op         string `json:"op"`
	}
	require.NoError(t, json.Unmarshal([]byte(n.Extra), &payload))
	require.Equal(t, id, payload.ID)
	require.Equal(t, entityType, payload.EntityType)
	require.Equal(t, op, payload.Op)
}

func TestGenericRepositoryWatchWithoutWatcher(t *testing.T) {
	ctx := t.Context()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[UserProfile](log, db)
	require.NoError(t, err)

	_, err = repo.Watch(ctx, uuid.NewV7().String())
	require.ErrorIs(t, err, pg.ErrWatchNotConfigured)
}

func TestGenericRepositoryUpsert(t *testing.T) {
	ctx := t.Context()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[UserProfile](log, db)
	require.NoError(t, err)
	userID := uuid.NewV7().String()

	profile := UserProfile{
		Name:    "Alice",
		Age:     30,
		Address: Address{City: "Munich", Country: "Germany"},
	}
	require.NoError(t, repo.Upsert(ctx, userID, profile))

	ent, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, int32(1), ent.Version)

	// Upsert with changed data replaces it and bumps the version
	changed := profile
	changed.Address.City = "Berlin"
	require.NoError(t, repo.Upsert(ctx, userID, changed))
	ent, err = repo.Get(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, int32(2), ent.Version)
	require.Equal(t, "Berlin", ent.Data.Address.City)

	// Upsert with identical data is a no-op: the version stays put
	require.NoError(t, repo.Upsert(ctx, userID, changed))
	ent, err = repo.Get(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, int32(2), ent.Version)

	// With a table per entity type, another entity type can use the same id
	// independently without interfering with the profile above.
	type OtherEntity struct {
		X int `json:"x"`
	}
	otherRepo, err := pg.NewGenericRepository[OtherEntity](log, db)
	require.NoError(t, err)
	require.NoError(t, otherRepo.Upsert(ctx, userID, OtherEntity{X: 1}))

	otherEnt, err := otherRepo.Get(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, 1, otherEnt.Data.X)

	// The profile row is untouched by the other entity type.
	ent, err = repo.Get(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, int32(2), ent.Version)
}

func TestGenericRepositoryPagination(t *testing.T) {
	ctx := t.Context()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[UserProfile](log, db)
	require.NoError(t, err)

	// Insert 5 entities sharing a common city so paging over the result is meaningful
	const total = 5
	for i := range total {
		err := repo.Create(ctx, uuid.NewV7().String(), UserProfile{
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

func TestGenericRepositoryIn(t *testing.T) {
	ctx := t.Context()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[UserProfile](log, db)
	require.NoError(t, err)

	for i, p := range []UserProfile{
		{Name: "Alice", Age: 30, Address: Address{City: "Berlin", Country: "Germany"}},
		{Name: "Bob", Age: 25, Address: Address{City: "Vienna", Country: "Austria"}},
		{Name: "Carol", Age: 40, Address: Address{City: "Munich", Country: "Germany"}},
	} {
		require.NoError(t, repo.Create(ctx, "id-"+strconv.Itoa(i), p))
	}

	names := func(ents []pg.Entity[UserProfile]) []string {
		out := make([]string, 0, len(ents))
		for _, e := range ents {
			out = append(out, e.Data.Name)
		}
		return out
	}

	// Multiple values match exactly the matching subset.
	results, err := repo.Query(ctx, []pg.QueryFilter{{Path: "name", Op: "IN", Value: []string{"Alice", "Carol"}}}, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Alice", "Carol"}, names(results))

	// A single value behaves like "=".
	results, err = repo.Query(ctx, []pg.QueryFilter{{Path: "name", Op: "IN", Value: []string{"Bob"}}}, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Bob"}, names(results))

	// The path may point into nested objects.
	results, err = repo.Query(ctx, []pg.QueryFilter{{Path: "address.country", Op: "IN", Value: []string{"Germany"}}}, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Alice", "Carol"}, names(results))

	// A value that matches nothing yields no rows.
	results, err = repo.Query(ctx, []pg.QueryFilter{{Path: "name", Op: "IN", Value: []string{"Nobody"}}}, nil)
	require.NoError(t, err)
	require.Empty(t, results)

	// An empty set matches no row (and must not produce invalid SQL).
	results, err = repo.Query(ctx, []pg.QueryFilter{{Path: "name", Op: "IN", Value: []string{}}}, nil)
	require.NoError(t, err)
	require.Empty(t, results)

	// A non-slice value is rejected.
	_, err = repo.Query(ctx, []pg.QueryFilter{{Path: "name", Op: "IN", Value: "Alice"}}, nil)
	require.Error(t, err)

	// The primary key can be addressed with "=" and "IN".
	results, err = repo.Query(ctx, []pg.QueryFilter{{Path: pg.PrimaryKeyPath, Op: "=", Value: "id-1"}}, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Bob"}, names(results))

	results, err = repo.Query(ctx, []pg.QueryFilter{{Path: pg.PrimaryKeyPath, Op: "IN", Value: []string{"id-0", "id-2", "id-9"}}}, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Alice", "Carol"}, names(results))

	results, err = repo.Query(ctx, []pg.QueryFilter{{Path: pg.PrimaryKeyPath, Op: "IN", Value: []string{}}}, nil)
	require.NoError(t, err)
	require.Empty(t, results)

	_, err = repo.Query(ctx, []pg.QueryFilter{{Path: pg.PrimaryKeyPath, Op: "LIKE", Value: "id-%"}}, nil)
	require.Error(t, err)

	// More distinct query shapes than the statement cache holds still work.
	for i := range 300 {
		filters := make([]pg.QueryFilter, 0, i+1)
		for range i + 1 {
			filters = append(filters, pg.QueryFilter{Path: "address.country", Op: "=", Value: "Germany"})
		}
		results, err = repo.Query(ctx, filters, nil)
		require.NoError(t, err)
		require.Len(t, results, 2)
	}
}

func TestGenericRepositoryLikeEscaping(t *testing.T) {
	ctx := t.Context()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[UserProfile](log, db)
	require.NoError(t, err)

	for i, name := range []string{"literal_1", "literalA1", "100%", "100x"} {
		require.NoError(t, repo.Create(ctx, "id-"+strconv.Itoa(i), UserProfile{Name: name}))
	}

	// `_` is a single-character wildcard; escaped it must match only the literal.
	results, err := repo.Query(ctx, []pg.QueryFilter{{Path: "name", Op: "LIKE", Value: "literal_1"}}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "literal_1", results[0].Data.Name)

	// `%` is a multi-character wildcard; escaped (here with ILIKE) it must match
	// only the literal.
	results, err = repo.Query(ctx, []pg.QueryFilter{{Path: "name", Op: "ILIKE", Value: "100%"}}, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "100%", results[0].Data.Name)

	// A caller-supplied wildcard is treated literally, so "literal%" matches
	// no row instead of both "literal"-prefixed names.
	results, err = repo.Query(ctx, []pg.QueryFilter{{Path: "name", Op: "LIKE", Value: "literal%"}}, nil)
	require.NoError(t, err)
	require.Empty(t, results)
}
