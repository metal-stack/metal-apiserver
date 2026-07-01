package generic

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_sharedMutex_stop(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan bool)

	mutex, err := newSharedMutex(context.Background(), slog.Default(), nil)
	require.NoError(t, err)

	go func() {
		mutex.expireloop(ctx)
		done <- true
	}()

	cancel()

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	select {
	case <-done:
	case <-timeoutCtx.Done():
		t.Errorf("shared mutex expiration did not stop")
	}
}
