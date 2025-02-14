package tx

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/metal-stack/api-server/pkg/test"
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

	var processedJobs []string

	tests := []struct {
		name              string
		tx                *Tx
		actionFns         actionFns
		wantProcessedJobs []string
		wantErr           bool
		wantPending       []Pending
	}{
		{
			name: "simple",
			tx:   &Tx{Jobs: []Job{{ID: "j1", Action: ActionIpDelete}}},
			actionFns: actionFns{ActionIpDelete: func(id string) error {
				log.Info("delete", "id", id)
				processedJobs = append(processedJobs, id)
				return nil
			}},
			wantProcessedJobs: []string{"j1"},
			wantErr:           false,
		},
		{
			name: "pending",
			tx:   &Tx{Jobs: alotJobs},
			actionFns: actionFns{ActionIpDelete: func(id string) error {
				if id == "j0" {
					log.Info("delete", "id", id)
					processedJobs = append(processedJobs, id)
					return nil
				}
				return fmt.Errorf("unable to process:%s", id)
			}},
			wantErr:           false,
			wantProcessedJobs: []string{"j0"},
			wantPending:       []Pending{{ID: "j2"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clear(processedJobs)
			ts, err := NewTxStore(ctx, log, client, tt.actionFns)
			require.NoError(t, err)

			if err := ts.AddTx(ctx, tt.tx); (err != nil) != tt.wantErr {
				t.Errorf("txStore.AddTx() error = %v, wantErr %v", err, tt.wantErr)
			}

			require.EqualValues(t, tt.wantProcessedJobs, processedJobs)

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

		})
	}
}
