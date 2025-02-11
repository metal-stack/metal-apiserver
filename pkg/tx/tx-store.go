package tx

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

const (
	stream = "metal-tx"
	group  = "txStore"
)

type (
	txStore struct {
		log              *slog.Logger
		client           *redis.Client
		messageIdToStart string
		actionFns        actionFns
	}

	actionFns map[Action]actionFn
	actionFn  func(id string) error
)

func NewTxStore(ctx context.Context, log *slog.Logger, client *redis.Client, actions actionFns) (*txStore, error) {
	result := client.XGroupCreateMkStream(ctx, stream, group, "$")
	if result.Err() != nil {
		return nil, result.Err()
	}

	return &txStore{
		log:              log,
		client:           client,
		actionFns:        actions,
		messageIdToStart: "0-0", // Start from beginning on startup, if set to ">" it starts with new unprocessed entries
	}, nil
}

func (t *txStore) AddTx(ctx context.Context, tx *Tx) error {
	serializedTx, err := json.Marshal(tx)
	if err != nil {
		return err
	}

	stringTx := base64.StdEncoding.EncodeToString(serializedTx) // do we need to encode or not?
	data := map[string]any{tx.Reference: stringTx}
	//we have received  an order here send it to
	//redis has a function called xadd that we will use to add this to our stream
	//you can read more about it on the link shared above.
	err = t.client.XAdd(ctx, &redis.XAddArgs{
		///this is the name we want to give to our stream
		///in our case we called it send_order_emails
		//note you can have as many stream as possible
		//such as one for email...another for notifications
		ID:     tx.Reference,
		Stream: stream,
		MaxLen: 0, //  means unlimited
		Approx: true,
		//values is the data you want to send to the stream
		//in our case we send a map with email and message keys
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
		data, err := t.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:   group,
			Streams: []string{stream, t.messageIdToStart},
			//count is number of entries we want to read from redis
			// Count: 4,
			//we use the block command to make sure if no entry is found we wait
			//until an entry is found
			Block: 0,
		}).Result()
		if err != nil {
			t.log.Error("unable to receive from stream", "error", err)
		}

		///we have received the data we should loop it and queue the messages
		//so that our jobs can start processing
		for _, result := range data {
			for _, message := range result.Messages {
				for txReference, txString := range message.Values {
					txJson, err := base64.StdEncoding.DecodeString(txString.(string))
					if err != nil {
						t.log.Error("unable to decode tx to json bytes", "tx reference", txReference, "error", err)
						continue
					}

					var tx Tx
					err = json.Unmarshal(txJson, &tx)
					if err != nil {
						t.log.Error("unable to unmarshal tx to json", "tx reference", txReference, "error", err)
						continue
					}

					err = t.processTx(tx)
					if err != nil {
						t.log.Error("unable to process tx", "tx reference", txReference, "error", err)
						continue
					}

					acked := t.client.XAck(ctx, stream, message.ID)
					if acked.Err() != nil {
						t.log.Error("tx could not be acked", "error", err)
					}
					t.messageIdToStart = message.ID
				}
			}
		}
	}
}

func (t *txStore) processTx(tx Tx) error {
	for _, job := range tx.Jobs {

		action, ok := t.actionFns[job.Action]
		if !ok {
			return fmt.Errorf("no action func defined for action:%s", job.Action)
		}
		err := action(job.ID)
		if err != nil {
			return fmt.Errorf("error executing action: %s with id: %s error: %w", job.Action, job.ID, err)
		}
	}

	return nil
}
