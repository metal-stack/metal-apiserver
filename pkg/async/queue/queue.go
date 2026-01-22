package queue

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type (
	MachineAllocationPayload struct {
		// UUID of the machine which was allocated and trigger the machine installation
		UUID string `json:"uuid,omitempty"`
	}
)

// Push allows adding an element to the queue which will then be delivered to the waiters.
// messages will only be received by one waiter which waits for the given key.
func Push[E any](ctx context.Context, log *slog.Logger, client *redis.Client, key string, elem E) error {
	encoded, err := json.Marshal(elem)
	if err != nil {
		return err
	}
	_, err = client.RPush(ctx, key, string(encoded)).Result()
	log.Debug("pushed", "key", key, "element", encoded)
	return err
}

// Wait receives elements from the queue with the given key, every elements in the queue not already consumed will be returned.
// when the context is done, the channel gets closed.
func Wait[E any](ctx context.Context, log *slog.Logger, client *redis.Client, key string) <-chan E {
	var (
		resChan = make(chan E)
		// timeout of 0 means blocking
		timeout = 0 * time.Second
	)

	go func() {
		for {
			log.Debug("waiting for queue entries", "key", key)

			// TODO consider BRPOPLPUSH and push the processed value into a new list
			// probably under key-processed to enable visibility
			// these entries must be removed after a certain timeout
			// this would require to have a timestamp in every message
			// see: https://valkey.io/commands/rpoplpush/ and https://valkey.io/commands/brpoplpush/
			results, err := client.BRPop(ctx, timeout, key).Result()
			if err != nil {
				select {
				case <-ctx.Done():
					close(resChan)
					return
				default:
					log.Error("error reading from queue", "error", err)
					continue
				}
			}

			log.Debug("wait received", "results", results)

			for _, result := range results {
				if result == key {
					continue
				}
				var elem E
				err = json.Unmarshal([]byte(result), &elem)
				if err != nil {
					log.Error("unable to decode result", "error", err)
					continue
				}

				resChan <- elem
			}
		}
	}()

	return resChan
}
