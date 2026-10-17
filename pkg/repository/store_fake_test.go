package repository_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/storage"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/stretchr/testify/require"
)

// fakeIPStorage is a hand-written storage.Storage[*metal.IP] returning canned
// results. It lets the store orchestrator (scope enforcement, error mapping and
// proto conversion) be exercised without a database container. Only Get and
// List are implemented; the embedded nil Storage satisfies the remaining
// methods for the code paths under test.
type fakeIPStorage struct {
	storage.Storage[*metal.IP]
	get  func(ctx context.Context, id string) (*metal.IP, error)
	list func(ctx context.Context, filters ...storage.Filter) ([]*metal.IP, error)
}

func (f *fakeIPStorage) Get(ctx context.Context, id string) (*metal.IP, error) {
	return f.get(ctx, id)
}

func (f *fakeIPStorage) List(ctx context.Context, filters ...storage.Filter) ([]*metal.IP, error) {
	return f.list(ctx, filters...)
}

// fakeDatastore overrides only IP(); the embedded nil Datastore satisfies the
// rest of the interface for the code paths under test.
type fakeDatastore struct {
	generic.Datastore
	ip storage.Storage[*metal.IP]
}

func (f *fakeDatastore) IP() storage.Storage[*metal.IP] { return f.ip }

func newFakeStore(ip storage.Storage[*metal.IP]) *repository.Store {
	return repository.New(repository.Config{
		Log:       slog.Default(),
		Datastore: &fakeDatastore{ip: ip},
	})
}

func TestStoreGetReturnsEntityFromStorage(t *testing.T) {
	ctx := t.Context()

	ip := &metal.IP{IPAddress: "10.0.0.1", ProjectID: "p1"}
	s := newFakeStore(&fakeIPStorage{
		get: func(_ context.Context, id string) (*metal.IP, error) {
			require.Equal(t, "10.0.0.1", id)
			return ip, nil
		},
	})

	got, err := s.IP("p1").Get(ctx, "10.0.0.1")
	require.NoError(t, err)
	require.Equal(t, "10.0.0.1", got.Ip)
	require.Equal(t, "p1", got.Project)
}

func TestStoreGetEnforcesProjectScope(t *testing.T) {
	ctx := t.Context()

	ip := &metal.IP{IPAddress: "10.0.0.1", ProjectID: "p2"}
	s := newFakeStore(&fakeIPStorage{
		get: func(context.Context, string) (*metal.IP, error) { return ip, nil },
	})

	_, err := s.IP("p1").Get(ctx, "10.0.0.1")
	require.Error(t, err)
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestStoreGetPropagatesStorageError(t *testing.T) {
	ctx := t.Context()

	storageErr := connect.NewError(connect.CodeUnavailable, errors.New("backend down"))
	s := newFakeStore(&fakeIPStorage{
		get: func(context.Context, string) (*metal.IP, error) { return nil, storageErr },
	})

	_, err := s.IP("p1").Get(ctx, "10.0.0.1")
	require.Error(t, err)
	require.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	require.Contains(t, err.Error(), "backend down")
}

func TestStoreListEnforcesProjectScope(t *testing.T) {
	ctx := t.Context()

	ips := []*metal.IP{
		{IPAddress: "10.0.0.1", ProjectID: "p1"},
		{IPAddress: "10.0.0.2", ProjectID: "p2"},
	}
	s := newFakeStore(&fakeIPStorage{
		list: func(context.Context, ...storage.Filter) ([]*metal.IP, error) { return ips, nil },
	})

	got, err := s.IP("p1").List(ctx, &apiv2.IPQuery{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "10.0.0.1", got[0].Ip)
}

func TestStoreUnscopedListReturnsAll(t *testing.T) {
	ctx := t.Context()

	ips := []*metal.IP{
		{IPAddress: "10.0.0.1", ProjectID: "p1"},
		{IPAddress: "10.0.0.2", ProjectID: "p2"},
	}
	s := newFakeStore(&fakeIPStorage{
		list: func(context.Context, ...storage.Filter) ([]*metal.IP, error) { return ips, nil },
	})

	got, err := s.UnscopedIP().List(ctx, nil)
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func TestStoreListPropagatesStorageError(t *testing.T) {
	ctx := t.Context()

	s := newFakeStore(&fakeIPStorage{
		list: func(context.Context, ...storage.Filter) ([]*metal.IP, error) {
			return nil, connect.NewError(connect.CodeInternal, errors.New("boom"))
		},
	})

	_, err := s.IP("p1").List(ctx, nil)
	require.Error(t, err)
	require.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}
