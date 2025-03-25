package async

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type (
	Notifier[E any] interface {
		// Notify allows adding an element to the stream which will then be delivered to the waiters.
		// messages fan out to all waiters.
		Notify(ctx context.Context, elem E) error
	}
	Waiter[E any] interface {
		// Wait receives elements from the stream starting from the point in time when this function is called.
		// when the context is done, the channel gets closed.
		Wait(ctx context.Context) <-chan E
	}

	notifier[E any] struct {
		client     *redis.Client
		streamName string
		log        *slog.Logger
	}

	waiter[E any] struct {
		client     *redis.Client
		streamName string
		log        *slog.Logger
	}
)

const (
	maxLength    = 1000
	elemAccessor = "e"
)

func New[E any](log *slog.Logger, client *redis.Client, streamNamePrefix string) (Notifier[E], Waiter[E]) {
	var elem E
	streamName := fmt.Sprintf("%s:%T", streamNamePrefix, elem)
	return &notifier[E]{
			log:        log.WithGroup("notify").With("stream-name", streamName),
			streamName: streamName,
			client:     client,
		}, &waiter[E]{
			log:        log.WithGroup("waiter").With("stream-name", streamName),
			streamName: streamName,
			client:     client,
		}
}

func (n *notifier[E]) Notify(ctx context.Context, elem E) error {
	encoded, err := json.Marshal(elem)
	if err != nil {
		return err
	}

	value := map[string]string{
		elemAccessor: string(encoded),
	}

	result, err := n.client.XAdd(ctx, &redis.XAddArgs{
		Stream: n.streamName,
		MaxLen: maxLength,
		Values: value,
	}).Result()
	if err != nil {
		return err
	}

	n.log.Info("notify", "id", result)

	return nil
}

func (w *waiter[E]) Wait(ctx context.Context) <-chan E {
	var (
		resChan = make(chan E)
		lastID  = "$"
	)

	go func() {
		for {
			w.log.Info("waiting for stream entries", "from-id", lastID)

			results, err := w.client.XRead(ctx, &redis.XReadArgs{
				Streams: []string{w.streamName},
				Block:   0,
				Count:   1,
				ID:      lastID,
			}).Result()
			if err != nil {
				select {
				case <-ctx.Done():
					close(resChan)
					return
				default:
					w.log.Error("error reading from stream", "error", err)
					time.Sleep(500 * time.Millisecond)
					continue
				}
			}

			w.log.Debug("received result", "result", results)

			if len(results) != 1 {
				w.log.Error("received more than a single result but used count 1, what is redis doing?")
				continue
			}

			result := results[0]

			if len(result.Messages) != 1 {
				w.log.Error("received more than a single message but used count 1, what is redis doing?")
				continue
			}

			m := result.Messages[0]

			lastID = m.ID
			w.log.Info("received message", "id", m.ID)

			encoded, ok := m.Values[elemAccessor]
			if !ok {
				w.log.Error("element is not contained in msg values")
				continue
			}

			encodedString, ok := encoded.(string)
			if !ok {
				w.log.Error("encoded element is malformed")
				continue
			}

			var elem E
			err = json.Unmarshal([]byte(encodedString), &elem)
			if err != nil {
				w.log.Error("unable to decode stream element", "error", err)
				continue
			}

			resChan <- elem
		}
	}()

	return resChan
}
