package tx

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/davecgh/go-spew/spew"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func Test_txStore_AddTx(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx := context.Background()
	tests := []struct {
		name      string
		tx        *Tx
		actionFns actionFns
		wantErr   bool
	}{
		{
			name: "simple",
			tx:   &Tx{Jobs: []Job{{ID: "j1", Action: ActionIpDelete}}},
			actionFns: actionFns{ActionIpDelete: func(id string) error {
				return nil
			}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

			tr, err := NewTxStore(ctx, log, client, tt.actionFns)
			require.NoError(t, err)

			if err := tr.AddTx(ctx, tt.tx); (err != nil) != tt.wantErr {
				t.Errorf("txStore.AddTx() error = %v, wantErr %v", err, tt.wantErr)
			}

			data, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: "txStore", Streams: []string{"metal-tx", ">"}}).Result()
			require.NoError(t, err)
			require.Len(t, data, 1)

			spew.Dump(data)
			// t.Fail()

			// go func() {
			// 	err := tr.Process()
			// 	if err != nil {
			// 		t.Fail()
			// 	}
			// }()
		})
	}
}
