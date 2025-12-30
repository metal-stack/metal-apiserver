package machinebmccommand

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/metal-stack/api/go/client"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
)

// TODO decide if we should move this to api, or to metal-bmc

type bmcServiceClient struct {
	client client.Client
	log    *slog.Logger
}

func NewClient(log *slog.Logger, client client.Client) *bmcServiceClient {
	return &bmcServiceClient{
		log:    log,
		client: client,
	}
}

// MessageHandler is called when a message is received
type MessageHandler func(*infrav2.WaitForBMCCommandResponse) error

// Subscribe subscribes to a topic and calls the handler for each message
func (c *bmcServiceClient) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	stream, err := c.client.Infrav2().BMC().WaitForBMCCommand(ctx, &infrav2.WaitForBMCCommandRequest{Partition: topic})
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}
	defer func() {
		_ = stream.Close()
	}()

	c.log.Info("subscribed to machine bmc command", "topic", topic)

	// Receive messages
	for stream.Receive() {
		msg := stream.Msg()
		if err := handler(msg); err != nil {
			c.log.Error("handler error", "error", err)
		}
	}

	if err := stream.Err(); err != nil {
		if err == io.EOF || err == context.Canceled {
			return nil
		}
		return fmt.Errorf("stream error: %w", err)
	}

	return nil
}

// SubscribeAsync subscribes asynchronously and returns a channel of messages
func (c *bmcServiceClient) SubscribeAsync(ctx context.Context, topic string) (<-chan *infrav2.WaitForBMCCommandResponse, <-chan error) {
	var (
		msgChan = make(chan *infrav2.WaitForBMCCommandResponse, 100)
		errChan = make(chan error, 1)
	)

	go func() {
		defer close(msgChan)
		defer close(errChan)

		err := c.Subscribe(ctx, topic, func(msg *infrav2.WaitForBMCCommandResponse) error {
			select {
			case msgChan <- msg:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})

		if err != nil && err != context.Canceled {
			errChan <- err
		}
	}()

	return msgChan, errChan
}
