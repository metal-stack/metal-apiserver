package queue_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/metal-apiserver/pkg/async/queue"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Queue(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	_, client, closer := test.StartValkey(t)
	defer func() {
		closer()
	}()

	tests := []struct {
		name       string
		machineIDs []string
	}{
		{
			name:       "add one machine",
			machineIDs: []string{"m-100"},
		},
		{
			name:       "add more machines",
			machineIDs: []string{"m-1", "m-2", "m-3", "m-4", "m-5", "m-6", "m-7", "m-8", "m-9", "fw-1"},
		},
		{
			name: "many machines",
			machineIDs: func() []string {
				var manyMachines []string
				for i := range 100 {
					manyMachines = append(manyMachines, fmt.Sprintf("happy-waiting-machine-%d", i))
				}
				return manyMachines
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				resultChan  = make(chan string)
				doneChan    = make(chan bool)
				wg          sync.WaitGroup
				gotMachines []string
				ctx, cancel = context.WithCancel(t.Context())
			)
			defer cancel()

			go func() {
				var resultCount int
				for m := range resultChan {
					log.Debug("got entry from resultChannel", "msg", m)
					gotMachines = append(gotMachines, m)
					resultCount++
					if len(tt.machineIDs) == resultCount {
						cancel()
					}
				}
				doneChan <- true
			}()

			for _, m := range tt.machineIDs {
				wg.Go(func() {
					for mid := range queue.Wait[task.MachineAllocationPayload](ctx, log, client, m) {
						t.Logf("waiter 1 got machineID: %s", mid)

						resultChan <- mid.UUID
					}
				})
			}

			for _, m := range tt.machineIDs {
				wg.Go(func() {
					for mid := range queue.Wait[task.MachineAllocationPayload](ctx, log, client, m) {
						t.Logf("waiter 2 got machineID: %s", mid)

						resultChan <- mid.UUID
					}
				})
			}

			for _, m := range tt.machineIDs {
				err := queue.Push(t.Context(), log, client, m, task.MachineAllocationPayload{UUID: m})
				require.NoError(t, err)
			}

			wg.Wait()
			close(resultChan)
			<-doneChan

			slices.Sort(tt.machineIDs)
			slices.Sort(gotMachines)

			assert.Len(t, gotMachines, len(tt.machineIDs))

			if diff := cmp.Diff(tt.machineIDs, gotMachines); diff != "" {
				t.Errorf("machines differ:%s", diff)
			}
		})
	}
}
