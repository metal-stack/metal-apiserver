package tx

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/metal-stack/api-server/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_txStore_AddTx(t *testing.T) {
	ctx := context.Background()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	container, client, err := test.StartValkey(t, ctx)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(ctx)
	}()

	var alotJobs []Job
	for i := range 2 {
		alotJobs = append(alotJobs, Job{
			ID:     fmt.Sprintf("j%d", i),
			Action: ActionIpDelete,
		})
	}

	processedJobs := make(chan string)
	actionDone := make(chan bool)

	tests := []struct {
		name              string
		tx                *Tx
		actionFn          ActionFn
		wantProcessedJobs []string
		wantErr           bool
		wantPending       []Pending
	}{
		{
			name: "simple ip",
			tx:   &Tx{Jobs: []Job{{ID: "j100", Action: ActionIpDelete}}},
			actionFn: func(ctx context.Context, job Job) error {
				id := job.ID
				log.Info("delete", "ip", id)
				processedJobs <- id
				actionDone <- true
				return nil
			},
			wantProcessedJobs: []string{"j100"},
			wantErr:           false,
		},
		{
			name: "simple network",
			tx:   &Tx{Jobs: []Job{{ID: "j200", Action: ActionNetworkDelete}}},
			actionFn: func(ctx context.Context, job Job) error {
				id := job.ID
				log.Info("delete", "network", id)
				processedJobs <- id
				actionDone <- true
				return nil
			},
			wantProcessedJobs: []string{"j200"},
			wantErr:           false,
		},
		{
			name: "one successful job",
			tx:   &Tx{Jobs: alotJobs},
			actionFn: func(ctx context.Context, job Job) error {
				id := job.ID
				log.Info("delete many", "id", id)
				if id == "j0" {
					processedJobs <- id
					actionDone <- true
					return nil
				}
				actionDone <- true
				return fmt.Errorf("unable to process:%s", id)
			},
			wantErr:           false,
			wantProcessedJobs: []string{"j0"},
			wantPending:       []Pending{{ID: "j0"}, {ID: "j1"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var processed []string
			ctx, cancel := context.WithCancel(ctx)
			ts, err := newTxStore(ctx, log, client, tt.actionFn)
			require.NoError(t, err)
			defer cancel()

			if err := ts.AddTx(ctx, tt.tx); (err != nil) != tt.wantErr {
				t.Errorf("txStore.AddTx() error = %v, wantErr %v", err, tt.wantErr)
			}

			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				jobid := <-processedJobs
				processed = append(processed, jobid)

				assert.ElementsMatch(c, tt.wantProcessedJobs, processed)
			}, time.Second, 100*time.Millisecond)

			// pending, err := ts.Pending(ctx)
			// assert.NoError(t, err)
			// require.ElementsMatch(t, tt.wantPending, pending)
			// t.Logf("pending:%#v", pending)

			// if len(tt.wantPending) > 0 {
			// 	pending, err := ts.Pending(ctx)
			// 	spew.Dump(pending)
			// 	require.NoError(t, err)
			// 	info, err := ts.Info(ctx)
			// 	require.NoError(t, err)
			// 	t.Logf("stream info:%#v", info)
			// 	t.Log(ts.Errors())
			// 	require.Equal(t, tt.wantPending, pending)
			// }
			<-actionDone
		})
	}
}
