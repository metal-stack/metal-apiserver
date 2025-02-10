package tx

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type Action string

var (
	defaultExpiration = 7 * 24 * time.Hour
)

const (
	ActionIpDelete Action = "ip-delete"
)

type Job struct {
	// ID is the unique identifier for the object to apply this job to
	// be aware that this should be an "allocation uuid" and not necessarily the primary key in the database of this object
	ID string `json:"id,omitempty"`

	// Action describes the action to be done when this job runs
	Action Action `json:"action,omitempty"`
}

type Tx struct {
	// Reference is the identifier of this transaction
	Reference string `json:"reference,omitempty"`

	// Jobs are the Jobs that are run in this transaction
	Jobs []Job `json:"jobs,omitempty"`

	Created    time.Time `json:"created,omitempty"`
	Expiration time.Time `json:"expiration,omitempty"`

	Activities []Activity `json:"activities,omitempty"`

	LastError error `json:"last_error,omitempty"`
}

type Activity struct {
	Timestamp time.Time `json:"timestamp,omitempty"`
	Message   string    `json:"message,omitempty"`
}

type Queue struct {
	log     *slog.Logger
	txStore *txStore
}

func New(log *slog.Logger, client *redis.Client) (*Queue, error) {
	ctx := context.Background()
	txStore, err := NewTxStore(ctx, log, client)
	if err != nil {
		return nil, err
	}
	return &Queue{
		log:     log,
		txStore: txStore,
	}, nil
}

func (q *Queue) Insert(ctx context.Context, jobs ...Job) error {
	now := time.Now()

	tx := &Tx{
		Jobs:       jobs,
		Created:    now,
		Expiration: now.Add(defaultExpiration),
	}

	return q.txStore.AddTx(ctx, tx)
}

func (q *Queue) List() []Tx {
	// TODO: redis scan

	return nil
}

func (q *Queue) Delete(ref string) error {
	return nil
}
