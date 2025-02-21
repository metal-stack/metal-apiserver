package tx

import (
	"context"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

const (
	ActionIpDelete      Action = "ip-delete"
	ActionNetworkDelete Action = "network-delete"
)

type (
	Action string

	ActionFn func(ctx context.Context, step Step) error

	Step struct {
		// ID is the unique identifier for the object to apply this job to
		// be aware that this should be an "allocation uuid" and not necessarily the primary key in the database of this object
		ID string `json:"id,omitempty"`

		// Action describes the action to be done when this job runs
		Action Action `json:"action,omitempty"`

		// Args might be necessary for complex Steps which require more query arguments than the primary ID of a Entity.
		Args map[string]any
	}

	Task struct {
		// Reference is the identifier of this transaction
		Reference string `json:"reference,omitempty"`

		// Steps are the Steps that are run in this transaction
		Steps []Step `json:"steps,omitempty"`

		LastError error `json:"last_error,omitempty"`
	}

	Tasks struct {
		log     *slog.Logger
		txStore *txStore
	}
)

func New(log *slog.Logger, client *redis.Client, actionFn ActionFn) (*Tasks, error) {
	ctx := context.Background()

	txStore, err := newTxStore(ctx, log, client, actionFn)
	if err != nil {
		return nil, err
	}
	return &Tasks{
		log:     log,
		txStore: txStore,
	}, nil
}

func (q *Tasks) Insert(ctx context.Context, tx *Task) error {
	return q.txStore.AddTx(ctx, tx)
}

func (q *Tasks) List() ([]Task, error) {
	pending, err := q.txStore.Pending(context.Background())
	if err != nil {
		return nil, err
	}
	var txs []Task
	for _, p := range pending {
		// FIXME
		txs = append(txs, Task{Reference: p.ID})
	}
	return txs, nil
}

func (q *Tasks) Delete(ref string) error {
	return nil
}
