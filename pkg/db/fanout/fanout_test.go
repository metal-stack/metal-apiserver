package fanout

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_notifier_NotifyAndWait(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	container, client, err := test.StartValkey(t, ctx)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(ctx)
	}()

	tests := []struct {
		name       string
		machineIDs []string
		wantErr    bool
	}{
		{
			name:       "add one machine",
			machineIDs: []string{"m-100"},
			wantErr:    false,
		},

		{
			name:       "add more machines",
			machineIDs: []string{"m-1", "m-2", "m-3", "m-4", "m-5", "m-6", "m-7", "m-8", "m-9", "fw-1"},
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			n := &notifier{
				client: client,
				log:    log,
			}
			w := &waiter{
				client: client,
				log:    log,
			}

			var machineIDs []string

			go func() {
				for mid := range w.Wait(ctx) {
					t.Logf("got machineID:%s", mid)
					machineIDs = append(machineIDs, mid)
					if len(machineIDs) == len(tt.machineIDs) {
						cancel()
					}
				}
			}()

			for _, m := range tt.machineIDs {
				if err := n.Notify(ctx, m); (err != nil) != tt.wantErr {
					t.Errorf("notifier.Notify() error = %v, wantErr %v", err, tt.wantErr)
				}
			}

			<-ctx.Done()
			assert.ElementsMatch(t, tt.machineIDs, machineIDs)

		})
	}
}
