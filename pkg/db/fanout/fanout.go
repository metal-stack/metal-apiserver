package fanout

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type (
	Notifier interface {
		// Notify is called from machine-service.go during machine/firewall create request
		Notify(ctx context.Context, machineID string) error
	}
	Waiter interface {
		// Wait is blocking and called from the wait-service.go which handles the streams from all metal-hammer instances.
		// Once a specific machineID is put into the fanout stream with Add(), every consumer of the Wait command will
		// receive the machineID to allocate in the machineIDChannel and must check if it has this machine is his waiting stream list.
		Wait(ctx context.Context) <-chan string
	}

	// Fanout interface {
	// 	Notifier
	// 	Waiter
	// }

	notifier struct {
		client *redis.Client
		log    *slog.Logger
	}

	waiter struct {
		client *redis.Client
		log    *slog.Logger
	}
)

const (
	streamname = "machine:allocation"
	maxlength  = 1000
)

func NewNotifier(log *slog.Logger, client *redis.Client) Notifier {

	return &notifier{
		log:    log,
		client: client,
	}
}

func NewWaiter(log *slog.Logger, client *redis.Client) Waiter {
	return &waiter{
		log:    log,
		client: client,
	}
}

func (n *notifier) Notify(ctx context.Context, machineID string) error {
	value := map[string]string{
		"machineID": machineID,
	}
	result, err := n.client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamname,
		MaxLen: maxlength,
		Values: value,
	}).Result()
	if err != nil {
		return err
	}
	n.log.Info("notify", "machineid", machineID, "result", result)
	return nil
}

func (w *waiter) Wait(ctx context.Context) <-chan string {
	machineIDChan := make(chan string)

	lastID := "$"
	go func() {
		for {
			w.log.Info("waiter: waiting for stream entries")

			result, err := w.client.XRead(ctx, &redis.XReadArgs{
				Streams: []string{streamname},
				Block:   0,
				Count:   1,
				ID:      lastID,
			}).Result()
			if err != nil {

				select {
				case <-ctx.Done():
					close(machineIDChan)
					return
				default:
					w.log.Error("error reading from stream", "error", err)
					time.Sleep(100 * time.Millisecond)
				}

			}
			w.log.Info("waiter", "result", result)

			if len(result) > 0 {
				for _, m := range result[0].Messages {
					lastID = m.ID
					w.log.Info("waiter", "message id", m.ID, "value", m.Values)

					machineID, ok := m.Values["machineID"]
					if ok {
						machineIDChan <- machineID.(string)
					}
				}
			}
		}
	}()

	return machineIDChan
}
