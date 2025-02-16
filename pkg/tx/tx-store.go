package tx

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	stream = "metal:tx"
	group  = "apiserver"
)

type (
	txStore struct {
		log              *slog.Logger
		client           *redis.Client
		messageIdToStart string
		actionFn         ActionFn
		processErrors    []error
	}
)

func newTxStore(ctx context.Context, log *slog.Logger, client *redis.Client, actionFn ActionFn) (*txStore, error) {
	// Check if group exists, create otherwise
	result := client.XGroupCreateMkStream(ctx, stream, group, "$")
	if result.Err() != nil && !strings.Contains(result.Err().Error(), "BUSYGROUP") {
		return nil, fmt.Errorf("xgroup create: %w", result.Err())
	}

	store := &txStore{
		log:              log,
		client:           client,
		actionFn:         actionFn,
		messageIdToStart: "0-0", // Start from beginning on startup, if set to ">" it starts with new unprocessed entries
	}
	go func() {
		err := store.Process(ctx)
		if err != nil {
			panic(err)
		}
	}()
	return store, nil
}

func (t *txStore) AddTx(ctx context.Context, tx *Tx) error {
	serializedTx, err := json.Marshal(tx)
	if err != nil {
		return err
	}

	stringTx := base64.StdEncoding.EncodeToString(serializedTx) // do we need to encode or not?
	data := map[string]any{tx.Reference: stringTx}
	err = t.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		MaxLen: 1000, // evict old entries, keep maximum 1000
		Approx: true,
		Values: data,
	}).Err()
	if err != nil {
		return fmt.Errorf("unable to enqueue transaction: %w", err)
	}

	return nil
}

func (t *txStore) Process(ctx context.Context) error {
	// TODO: read the history of unprocessed jobs with 0, then just tail unprocessed jobs with >

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		data, err := t.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Streams:  []string{stream, ">"},
			Consumer: "me",
			NoAck:    true,
			//count is number of entries we want to read from redis
			Count: 1,
			//we use the block command to make sure if no entry is found we wait
			//until an entry is found
			Block: 0,
		}).Result()
		if err != nil {
			t.log.Error("unable to receive from stream", "error", err)
			t.processErrors = append(t.processErrors, err)
			continue
		}

		///we have received the data we should loop it and queue the messages
		//so that our jobs can start processing
		for _, result := range data {
			for _, message := range result.Messages {
				for txReference, txString := range message.Values {
					txJson, err := base64.StdEncoding.DecodeString(txString.(string))
					if err != nil {
						t.log.Error("unable to decode tx to json bytes", "tx reference", txReference, "error", err)
						t.processErrors = append(t.processErrors, err)
						continue
					}

					var tx Tx
					err = json.Unmarshal(txJson, &tx)
					if err != nil {
						t.log.Error("unable to unmarshal tx to json", "tx reference", txReference, "error", err)
						t.processErrors = append(t.processErrors, err)
						continue
					}

					err = t.processTx(ctx, tx)
					if err != nil {
						t.log.Error("unable to process tx", "tx reference", txReference, "error", err)
						t.processErrors = append(t.processErrors, err)
						continue
					}

					acked := t.client.XAck(ctx, stream, group, message.ID)
					if acked.Err() != nil {
						t.log.Error("tx could not be acked", "error", acked.Err())
						t.processErrors = append(t.processErrors, acked.Err())
					}
					t.messageIdToStart = message.ID
				}
			}
		}
	}
}

func (t *txStore) processTx(ctx context.Context, tx Tx) error {
	var errs []error
	for _, job := range tx.Jobs {
		err := t.actionFn(ctx, job)
		if err != nil {
			errs = append(errs, fmt.Errorf("error executing action: %s with id: %s error: %w", job.Action, job.ID, err))
			continue
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

type Pending struct {
	ID         string
	Consumer   string
	Idle       time.Duration
	RetryCount int64
}

func (t *txStore) Pending(ctx context.Context) ([]Pending, error) {
	pendingStreams, err := t.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: stream,
		Group:  group,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	var pendingTxs []Pending
	for _, stream := range pendingStreams {
		pendingTxs = append(pendingTxs, Pending{
			ID:         stream.ID,
			Consumer:   stream.Consumer,
			Idle:       stream.Idle,
			RetryCount: stream.RetryCount,
		})
	}
	return pendingTxs, nil
}

func (t *txStore) Info(ctx context.Context) (*redis.XInfoStream, error) {
	streamInfo, err := t.client.XInfoStream(ctx, stream).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	return streamInfo, nil
}

func (t *txStore) Errors() error {
	return errors.Join(t.processErrors...)
}
