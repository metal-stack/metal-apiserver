package async_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/async"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/assert"
)

func Test_notifier_NotifyAndWait(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client, closer := test.StartValkey(t)
	defer func() {
		closer()
	}()

	oneThousandMachines := make([]string, 0, 1000)
	for i := range 1000 {
		oneThousandMachines = append(oneThousandMachines, fmt.Sprintf("happy-waiting-machine-%d", i))
	}

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
		{
			name:       "add a thousand machines",
			machineIDs: oneThousandMachines,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx1, cancel1 := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel1()
			ctx2, cancel2 := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel2()

			n, w1 := async.New[string](log, client, tt.name)
			_, w2 := async.New[string](log, client, tt.name)

			var (
				waiter1machineIDs []string
				waiter2machineIDs []string
			)

			go func() {
				for mid := range w1.Wait(ctx1) {
					t.Logf("waiter 1 got machineID: %s", mid)

					waiter1machineIDs = append(waiter1machineIDs, mid)

					if len(waiter1machineIDs) == len(tt.machineIDs) {
						cancel1()
					}
				}
			}()

			go func() {
				for mid := range w2.Wait(ctx2) {
					t.Logf("waiter 2 got machineID: %s", mid)

					waiter2machineIDs = append(waiter2machineIDs, mid)

					if len(waiter2machineIDs) == len(tt.machineIDs) {
						cancel2()
					}
				}
			}()

			time.Sleep(100 * time.Millisecond) // this one's important!

			for _, m := range tt.machineIDs {
				if err := n.Notify(t.Context(), m); (err != nil) != tt.wantErr {
					t.Errorf("notifier.Notify() error = %v, wantErr %v", err, tt.wantErr)
				}
			}

			<-ctx1.Done()
			<-ctx2.Done()

			assert.ElementsMatch(t, tt.machineIDs, waiter1machineIDs)
			assert.ElementsMatch(t, tt.machineIDs, waiter2machineIDs)
		})
	}
}
