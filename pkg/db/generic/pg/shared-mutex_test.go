package pg_test

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_sharedMutex_reallyLocking(t *testing.T) {
	t.Parallel()
	var (
		log        = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx        = t.Context()
		expiration = pg.NewLockOptExpirationTimeout(10 * time.Second)
	)

	db, closer := test.StartPostgres(t, log)
	defer closer()

	mutex, err := pg.NewSharedMutex(ctx, log, db)
	require.NoError(t, err)

	err = mutex.Lock(ctx, "test", expiration, pg.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.NoError(t, err)

	err = mutex.Lock(ctx, "test", expiration, pg.NewLockOptAcquireTimeout(50*time.Millisecond))
	require.Error(t, err)
	require.ErrorContains(t, err, "unable to acquire mutex")

	err = mutex.Lock(ctx, "test2", expiration, pg.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.NoError(t, err)

	err = mutex.Lock(ctx, "test", expiration, pg.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.Error(t, err)
	require.ErrorContains(t, err, "unable to acquire mutex")

	mutex.Unlock(ctx, "test")

	err = mutex.Lock(ctx, "test2", expiration, pg.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.Error(t, err)
	require.ErrorContains(t, err, "unable to acquire mutex")

	err = mutex.Lock(ctx, "test", expiration, pg.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.NoError(t, err)
}

func Test_sharedMutex_acquireAfterRelease(t *testing.T) {
	t.Parallel()
	var (
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx = t.Context()
	)

	db, closer := test.StartPostgres(t, log)
	defer closer()

	mutex, err := pg.NewSharedMutex(ctx, log, db)
	require.NoError(t, err)

	err = mutex.Lock(ctx, "test", pg.NewLockOptExpirationTimeout(3*time.Second), pg.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Go(func() {
		err = mutex.Lock(ctx, "test", pg.NewLockOptExpirationTimeout(1*time.Second), pg.NewLockOptAcquireTimeout(3*time.Second))
		assert.NoError(t, err)
	})

	time.Sleep(1 * time.Second)

	mutex.Unlock(ctx, "test")

	wg.Wait()
}

func Test_sharedMutex_expires(t *testing.T) {
	t.Parallel()
	var (
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx = t.Context()
	)

	db, closer := test.StartPostgres(t, log)
	defer closer()

	// run the expiration loop frequently so the test does not wait for the
	// default check interval
	mutex, err := pg.NewSharedMutex(ctx, log, db, pg.NewMutexOptCheckInterval(100*time.Millisecond))
	require.NoError(t, err)

	err = mutex.Lock(ctx, "test", pg.NewLockOptExpirationTimeout(2*time.Second), pg.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.NoError(t, err)

	err = mutex.Lock(ctx, "test", pg.NewLockOptExpirationTimeout(2*time.Second), pg.NewLockOptAcquireTimeout(100*time.Millisecond))
	require.Error(t, err)
	require.ErrorContains(t, err, "unable to acquire mutex")

	done := make(chan bool)
	go func() {
		err = mutex.Lock(ctx, "test", pg.NewLockOptExpirationTimeout(2*time.Second), pg.NewLockOptAcquireTimeout(6*time.Second))
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
