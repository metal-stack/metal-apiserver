package tx

import (
	"log/slog"
	"time"
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
	ID string

	// Action describes the action to be done when this job runs
	Action Action
}

type Tx struct {
	// Reference is the identifier of this transaction
	Reference string

	// jobs are the jobs that are run in this transaction
	jobs []Job

	Created    time.Time
	Expiration time.Time

	Activities []Activity

	LastError error
}

type Activity struct {
	Timestamp time.Time
	Message   string
}

type Queue struct {
	log   *slog.Logger
	redis any
}

func New(log *slog.Logger, redis any) *Queue {
	return &Queue{
		log:   log,
		redis: redis,
	}
}

func (q *Queue) Insert(jobs ...Job) error {
	now := time.Now()

	_ = &Tx{
		jobs:       jobs,
		Created:    now,
		Expiration: now.Add(defaultExpiration),
	}

	// TODO: redis.Add(tx)

	return nil
}

func (q *Queue) List() []Tx {
	// TODO: redis scan

	return nil
}

func (q *Queue) Delete(ref string) error {
	return nil
}

func (q *Queue) run() {
	actions := make(chan Action)

	for {
		select {
		case action := <-actions:
			switch action {
			case ActionIpDelete:

			}
		}
	}
}

func ipDelete() error {
	return nil
}
