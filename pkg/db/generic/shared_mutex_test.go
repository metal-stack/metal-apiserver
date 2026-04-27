package generic_test

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_sharedMutex_reallyLocking(t *testing.T) {
	t.Parallel()
	var (
		log        = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx        = t.Context()
		expiration = generic.NewLockOptExpirationTimeout(10 * time.Second)
	)

	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	err := ds.Lock(ctx, "test", expiration, generic.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.NoError(t, err)

	err = ds.Lock(ctx, "test", expiration, generic.NewLockOptAcquireTimeout(50*time.Millisecond))
	require.Error(t, err)
	require.ErrorContains(t, err, "unable to acquire mutex")

	err = ds.Lock(ctx, "test2", expiration, generic.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.NoError(t, err)

	err = ds.Lock(ctx, "test", expiration, generic.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.Error(t, err)
	require.ErrorContains(t, err, "unable to acquire mutex")

	ds.Unlock(ctx, "test")

	err = ds.Lock(ctx, "test2", expiration, generic.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.Error(t, err)
	require.ErrorContains(t, err, "unable to acquire mutex")

	err = ds.Lock(ctx, "test", expiration, generic.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.NoError(t, err)
}

func Test_sharedMutex_acquireAfterRelease(t *testing.T) {
	t.Parallel()
	var (
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx = t.Context()
	)

	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	err := ds.Lock(ctx, "test", generic.NewLockOptExpirationTimeout(3*time.Second), generic.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Go(func() {
		err = ds.Lock(ctx, "test", generic.NewLockOptExpirationTimeout(1*time.Second), generic.NewLockOptAcquireTimeout(3*time.Second))
		assert.NoError(t, err)
	})

	time.Sleep(1 * time.Second)

	ds.Unlock(ctx, "test")

	wg.Wait()
}

func Test_sharedMutex_expires(t *testing.T) {
	t.Parallel()
	var (
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx = t.Context()
	)

	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	err := ds.Lock(ctx, "test", generic.NewLockOptExpirationTimeout(2*time.Second), generic.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.NoError(t, err)

	err = ds.Lock(ctx, "test", generic.NewLockOptExpirationTimeout(2*time.Second), generic.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.Error(t, err)
	require.ErrorContains(t, err, "unable to acquire mutex")

	done := make(chan bool)
	go func() {
		err = ds.Lock(ctx, "test", generic.NewLockOptExpirationTimeout(2*time.Second), generic.NewLockOptAcquireTimeout(6*time.Second))
		if err != nil {
			t.Errorf("mutex was not acquired: %s", err)
		}
		done <- true
	}()

	timeoutCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	select {
	case <-done:
	case <-timeoutCtx.Done():
		t.Errorf("shared mutex has not expired")
	}
}
