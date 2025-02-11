package tx

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	// "github.com/alicebob/miniredis/v2"
	"github.com/davecgh/go-spew/spew"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func Test_txStore_AddTx(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var alotJobs []Job
	for i := range 2 {
		alotJobs = append(alotJobs, Job{
			ID:     fmt.Sprintf("j%d", i),
			Action: ActionIpDelete,
		})
	}

	ctx := context.Background()
	tests := []struct {
		name        string
		tx          *Tx
		actionFns   actionFns
		wantErr     bool
		wantPending []Pending
	}{
		{
			name: "simple",
			tx:   &Tx{Jobs: []Job{{ID: "j1", Action: ActionIpDelete}}},
			actionFns: actionFns{ActionIpDelete: func(id string) error {
				t.Logf("process job with id:%s", id)
				return nil
			}},
			wantErr: false,
		},
		{
			name: "pending",
			tx:   &Tx{Jobs: alotJobs},
			actionFns: actionFns{ActionIpDelete: func(id string) error {
				if id == "j1" {
					return nil
				}
				return fmt.Errorf("unable to process:%s", id)
			}},
			wantErr:     false,
			wantPending: []Pending{{ID: "j2"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			realRedis := "localhost:6379"
			// mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: realRedis})

			ts, err := NewTxStore(ctx, log, client, tt.actionFns)
			require.NoError(t, err)

			if err := ts.AddTx(ctx, tt.tx); (err != nil) != tt.wantErr {
				t.Errorf("txStore.AddTx() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(tt.wantPending) > 0 {
				pending, err := ts.Pending(ctx)
				spew.Dump(pending)
				require.NoError(t, err)
				info, err := ts.Info(ctx)
				require.NoError(t, err)
				t.Logf("stream info:%#v", info)
				t.Log(ts.Errors())
				require.Equal(t, tt.wantPending, pending)
			}

		})
	}
}
