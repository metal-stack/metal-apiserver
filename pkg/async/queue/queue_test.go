package queue_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/metal-apiserver/pkg/async/queue"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

func Test_Queue(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client, closer := test.StartValkey(t)
	defer func() {
		closer()
	}()

	var fiftyMachines []string
	for i := range 100 {
		fiftyMachines = append(fiftyMachines, fmt.Sprintf("happy-waiting-machine-%d", i))
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
			name:       "fifty machines",
			machineIDs: fiftyMachines,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				resultChan  = make(chan string)
				wg          sync.WaitGroup
				gotMachines []string
			)

			go func() {
				for m := range resultChan {
					gotMachines = append(gotMachines, m)
				}
			}()

			for _, m := range tt.machineIDs {
				wg.Go(func() {
					ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
					defer cancel()
					for mid := range queue.Wait[queue.MachineAllocationPayload](ctx, log, client, m) {
						t.Logf("waiter 1 got machineID: %s", mid)

						resultChan <- mid.UUID
					}
				})
			}

			for _, m := range tt.machineIDs {
				wg.Go(func() {
					ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
					defer cancel()
					for mid := range queue.Wait[queue.MachineAllocationPayload](ctx, log, client, m) {
						t.Logf("waiter 2 got machineID: %s", mid)

						resultChan <- mid.UUID
					}
				})
			}

			for _, m := range tt.machineIDs {
				if err := queue.Push(t.Context(), log, client, m, queue.MachineAllocationPayload{UUID: m}); (err != nil) != tt.wantErr {
					t.Errorf("queue.Push() error = %v, wantErr %v", err, tt.wantErr)
				}
			}

			time.Sleep(1000 * time.Millisecond) // this one's important!

			wg.Wait()
			close(resultChan)

			slices.Sort(tt.machineIDs)
			slices.Sort(gotMachines)

			require.ElementsMatch(t, tt.machineIDs, gotMachines)
			
			if diff := cmp.Diff(tt.machineIDs, gotMachines); diff != "" {
				t.Errorf("machines differ:%s", diff)
			}
		})
	}
}
