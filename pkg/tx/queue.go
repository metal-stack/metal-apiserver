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

	ActionFn func(ctx context.Context, job Job) error

	Job struct {
		// ID is the unique identifier for the object to apply this job to
		// be aware that this should be an "allocation uuid" and not necessarily the primary key in the database of this object
		ID string `json:"id,omitempty"`

		// Action describes the action to be done when this job runs
		Action Action `json:"action,omitempty"`
	}

	Tx struct {
		// Reference is the identifier of this transaction
		Reference string `json:"reference,omitempty"`

		// Jobs are the Jobs that are run in this transaction
		Jobs []Job `json:"jobs,omitempty"`

		LastError error `json:"last_error,omitempty"`
	}

	Queue struct {
		log     *slog.Logger
		txStore *txStore
	}
)

func New(log *slog.Logger, client *redis.Client, actionFn ActionFn) (*Queue, error) {
	ctx := context.Background()

	txStore, err := newTxStore(ctx, log, client, actionFn)
	if err != nil {
		return nil, err
	}
	return &Queue{
		log:     log,
		txStore: txStore,
	}, nil
}

func (q *Queue) Insert(ctx context.Context, tx *Tx) error {
	return q.txStore.AddTx(ctx, tx)
}

func (q *Queue) List() ([]Tx, error) {
	pending, err := q.txStore.Pending(context.Background())
	if err != nil {
		return nil, err
	}
	var txs []Tx
	for _, p := range pending {
		// FIXME
		txs = append(txs, Tx{Reference: p.ID})
	}
	return txs, nil
}

func (q *Queue) Delete(ref string) error {
	return nil
}
