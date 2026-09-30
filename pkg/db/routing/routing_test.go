package routing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/stretchr/testify/require"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

// noopRethinkFilter is a valid RethinkDB filter representation for tests.
func noopRethinkFilter() generic.EntityQuery {
	return func(q r.Term) r.Term { return q }
}

type testEntity struct {
	ID         string
	Created    time.Time
	Changed    time.Time
	Generation uint64
}

func (e *testEntity) GetID() string         { return e.ID }
func (e *testEntity) SetID(id string)       { e.ID = id }
func (e *testEntity) GetChanged() time.Time { return e.Changed }
func (e *testEntity) GetCreated() time.Time { return e.Created }
func (e *testEntity) GetGeneration() uint64 { return e.Generation }

func newTestEntity(id string) *testEntity {
	return &testEntity{
		ID:         id,
		Created:    time.Now().Add(-time.Hour),
		Changed:    time.Now().Add(-time.Hour),
		Generation: 3,
	}
}

type fakeStorage struct {
	name string

	calls []string

	// mutateOnUpdate simulates the rethinkdb adapter stamping changed/generation
	// in place.
	mutateOnUpdate bool

	err        error
	createRes  *testEntity
	getRes     *testEntity
	findRes    *testEntity
	listRes    []*testEntity
	lastEntity *testEntity
	lastID     string
	lastFilter []Filter
}

var _ Storage[*testEntity] = (*fakeStorage)(nil)

func (f *fakeStorage) Create(_ context.Context, e *testEntity) (*testEntity, error) {
	f.record("create", e)
	if f.err != nil {
		return nil, f.err
	}
	if f.createRes != nil {
		return f.createRes, nil
	}
	return e, nil
}

func (f *fakeStorage) Update(_ context.Context, e *testEntity) error {
	f.record("update", e)
	if f.err != nil {
		return f.err
	}
	if f.mutateOnUpdate {
		e.Changed = time.Now()
		e.Generation++
	}
	return nil
}

func (f *fakeStorage) Upsert(_ context.Context, e *testEntity) error {
	f.record("upsert", e)
	if f.err != nil {
		return f.err
	}
	if f.mutateOnUpdate {
		e.Generation++
	}
	return nil
}

func (f *fakeStorage) Delete(_ context.Context, e *testEntity) error {
	f.record("delete", e)
	return f.err
}

func (f *fakeStorage) Get(_ context.Context, id string) (*testEntity, error) {
	f.calls = append(f.calls, "get")
	f.lastID = id
	if f.err != nil {
		return nil, f.err
	}
	return f.getRes, nil
}

func (f *fakeStorage) Find(_ context.Context, filters ...Filter) (*testEntity, error) {
	f.calls = append(f.calls, "find")
	f.lastFilter = filters
	if f.err != nil {
		return nil, f.err
	}
	return f.findRes, nil
}

func (f *fakeStorage) List(_ context.Context, filters ...Filter) ([]*testEntity, error) {
	f.calls = append(f.calls, "list")
	f.lastFilter = filters
	if f.err != nil {
		return nil, f.err
	}
	return f.listRes, nil
}

func (f *fakeStorage) record(op string, e *testEntity) {
	f.calls = append(f.calls, op)
	f.lastEntity = e
}

func newRouter(t *testing.T, cfg Config, rethink, postgres *fakeStorage) *Router[*testEntity] {
	t.Helper()
	var (
		r Storage[*testEntity]
		p Storage[*testEntity]
	)
	if rethink != nil {
		r = rethink
	}
	if postgres != nil {
		p = postgres
	}
	router, err := NewRouter(nil, cfg, "Machine", r, p)
	require.NoError(t, err)
	return router
}

func TestRouterRequiresBackendsForMode(t *testing.T) {
	_, err := NewRouter(nil, Config{Default: ModeRethink}, "Machine", nil, &fakeStorage{})
	require.ErrorContains(t, err, "no rethinkdb backend")

	_, err = NewRouter(nil, Config{Default: ModePostgres}, "Machine", &fakeStorage{}, nil)
	require.ErrorContains(t, err, "no postgres backend")

	_, err = NewRouter(nil, Config{Default: ModeBoth}, "Machine", &fakeStorage{}, nil)
	require.ErrorContains(t, err, "both backends are required")

	_, err = NewRouter(nil, Config{Entities: map[string]Mode{"Machine": "mysql"}}, "Machine", &fakeStorage{}, &fakeStorage{})
	require.Error(t, err)
}

func TestRouterRethinkOnly(t *testing.T) {
	rethink := &fakeStorage{name: "rethink", mutateOnUpdate: true}
	router := newRouter(t, Config{}, rethink, nil)

	e := newTestEntity("id-1")
	_, err := router.Create(context.Background(), e)
	require.NoError(t, err)
	require.NoError(t, router.Update(context.Background(), e))
	require.NoError(t, router.Delete(context.Background(), e))

	require.Equal(t, []string{"create", "update", "delete"}, rethink.calls)
	require.Equal(t, ModeRethink, router.Mode())
}

func TestRouterPostgresOnly(t *testing.T) {
	postgres := &fakeStorage{name: "postgres"}
	router := newRouter(t, Config{Default: ModePostgres}, nil, postgres)

	e := newTestEntity("id-1")
	_, err := router.Create(context.Background(), e)
	require.NoError(t, err)
	require.NoError(t, router.Update(context.Background(), e))

	require.Equal(t, []string{"create", "update"}, postgres.calls)
	require.Equal(t, ModePostgres, router.Mode())
}

func TestRouterBothWritesToBoth(t *testing.T) {
	rethink := &fakeStorage{name: "rethink", mutateOnUpdate: true}
	postgres := &fakeStorage{name: "postgres"}
	router := newRouter(t, Config{Default: ModeBoth}, rethink, postgres)
	ctx := context.Background()

	e := newTestEntity("id-1")
	_, err := router.Create(ctx, e)
	require.NoError(t, err)
	require.Equal(t, []string{"create"}, rethink.calls)
	require.Equal(t, []string{"create"}, postgres.calls)

	require.NoError(t, router.Update(ctx, e))
	require.Equal(t, []string{"create", "update"}, rethink.calls)
	require.Equal(t, []string{"create", "update"}, postgres.calls)

	require.NoError(t, router.Delete(ctx, e))
	require.Equal(t, []string{"create", "update", "delete"}, rethink.calls)
	require.Equal(t, []string{"create", "update", "delete"}, postgres.calls)
}

// TestRouterBothMirrorsPreUpdateGeneration verifies that the postgres mirror
// receives the entity state before the rethinkdb adapter mutates it in place.
// This is what keeps the generation↔version mapping correct.
func TestRouterBothMirrorsPreUpdateGeneration(t *testing.T) {
	rethink := &fakeStorage{name: "rethink", mutateOnUpdate: true}
	postgres := &fakeStorage{name: "postgres"}
	router := newRouter(t, Config{Default: ModeBoth}, rethink, postgres)

	e := newTestEntity("id-1")
	e.Generation = 3
	require.NoError(t, router.Update(context.Background(), e))

	require.Equal(t, uint64(3), postgres.lastEntity.Generation, "mirror sees the pre-update generation")
	require.Equal(t, uint64(4), e.Generation, "primary mutates the caller's entity")
	require.NotSame(t, postgres.lastEntity, rethink.lastEntity, "backends must not share the entity pointer")
}

func TestRouterBothPartialWrite(t *testing.T) {
	rethink := &fakeStorage{name: "rethink"}
	postgres := &fakeStorage{name: "postgres", err: errors.New("postgres down")}
	router := newRouter(t, Config{Default: ModeBoth}, rethink, postgres)

	e := newTestEntity("id-1")
	_, err := router.Create(context.Background(), e)
	require.ErrorIs(t, err, ErrPartialWrite)
	require.Equal(t, []string{"create"}, rethink.calls, "primary write still happened")

	require.ErrorIs(t, router.Delete(context.Background(), e), ErrPartialWrite)
}

func TestRouterReadsPrimary(t *testing.T) {
	rethink := &fakeStorage{name: "rethink", getRes: newTestEntity("from-rethink")}
	postgres := &fakeStorage{name: "postgres", getRes: newTestEntity("from-postgres")}

	// default read backend is rethinkdb
	router := newRouter(t, Config{Default: ModeBoth}, rethink, postgres)
	got, err := router.Get(context.Background(), "id-1")
	require.NoError(t, err)
	require.Equal(t, "from-rethink", got.ID)
	require.Equal(t, []string{"get"}, rethink.calls)
	require.Empty(t, postgres.calls)

	// explicit read backend postgres
	router = newRouter(t, Config{Default: ModeBoth, ReadFrom: ModePostgres}, rethink, postgres)
	got, err = router.Get(context.Background(), "id-1")
	require.NoError(t, err)
	require.Equal(t, "from-postgres", got.ID)
	require.Equal(t, []string{"get"}, rethink.calls, "rethinkdb was only read by the first router")
	require.Equal(t, []string{"get"}, postgres.calls)
}

func TestRouterRequiresFilterForReadBackend(t *testing.T) {
	rethinkOnlyFilter := Filter{Rethink: noopRethinkFilter()}
	postgresOnlyFilter := Filter{Postgres: []pg.QueryFilter{{Path: "name", Op: "=", Value: "x"}}}

	rethink := &fakeStorage{name: "rethink"}
	postgres := &fakeStorage{name: "postgres"}

	// reading from rethinkdb with a postgres-only filter must fail
	router := newRouter(t, Config{Default: ModeBoth}, rethink, postgres)
	_, err := router.List(context.Background(), postgresOnlyFilter)
	require.ErrorIs(t, err, ErrFilterUnsupported)

	// reading from postgres with a rethink-only filter must fail
	router = newRouter(t, Config{Default: ModeBoth, ReadFrom: ModePostgres}, rethink, postgres)
	_, err = router.List(context.Background(), rethinkOnlyFilter)
	require.ErrorIs(t, err, ErrFilterUnsupported)
}

func TestEntityUUID(t *testing.T) {
	e := newTestEntity("")
	id, err := entityUUID(e)
	require.NoError(t, err)
	require.NotEmpty(t, e.ID, "an empty id is generated and written back")
	require.Equal(t, e.ID, id.String())

	_, err = entityUUID(newTestEntity("not-a-uuid"))
	require.ErrorContains(t, err, "non-uuid id")
}

func TestStampHelpers(t *testing.T) {
	t.Run("create stamps created/changed/zero generation", func(t *testing.T) {
		e := newTestEntity("id-1")
		require.NoError(t, stampCreate(e))
		require.False(t, e.Created.IsZero())
		require.False(t, e.Changed.IsZero())
		require.Equal(t, uint64(0), e.Generation)
	})

	t.Run("update increments generation", func(t *testing.T) {
		e := newTestEntity("id-1")
		e.Generation = 5
		require.NoError(t, stampUpdate(e))
		require.Equal(t, uint64(6), e.Generation)
	})

	t.Run("upsert keeps an existing created timestamp", func(t *testing.T) {
		created := time.Now().Add(-time.Hour).Round(time.Second)
		e := newTestEntity("id-1")
		e.Created = created
		e.Generation = 5
		require.NoError(t, stampUpsert(e))
		require.Equal(t, created, e.Created)
		require.Equal(t, uint64(6), e.Generation)
	})

	t.Run("upsert stamps a zero created timestamp", func(t *testing.T) {
		e := newTestEntity("id-1")
		e.Created = time.Time{}
		require.NoError(t, stampUpsert(e))
		require.False(t, e.Created.IsZero())
	})
}

func TestMapPostgresError(t *testing.T) {
	require.NoError(t, mapPostgresError(nil))
	require.ErrorIs(t, mapPostgresError(pg.ErrNotFound), ErrNotFound)
	require.ErrorIs(t, mapPostgresError(pg.ErrAlreadyExists), ErrAlreadyExists)
	require.ErrorIs(t, mapPostgresError(pg.ErrOptimisticLockConflict), ErrConflict)

	other := errors.New("boom")
	require.Equal(t, other, mapPostgresError(other))
}
