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

// Push allows adding an value to the queue which will then be delivered to the waiters.
// messages will only be received by one waiter which waits for the given queueName.
func Push(ctx context.Context, log *slog.Logger, client *redis.Client, queueName string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = client.RPush(ctx, queueName, string(encoded)).Result()
	log.Debug("pushed", "queueName", queueName, "value", encoded)
	return err
}

// Wait receives values from the queue with the given queueName, every values in the queue not already consumed will be returned.
// when the context is done, the channel gets closed.
func Wait[E any](ctx context.Context, log *slog.Logger, client *redis.Client, queueName string) <-chan E {
	var (
		resChan = make(chan E)
		// timeout of 0 means blocking
		timeout = 0 * time.Second
	)

	go func() {
		for {
			log.Debug("waiting for queue entries", "queueName", queueName)
			results, err := client.BRPop(ctx, timeout, queueName).Result()
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
				if result == queueName {
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
