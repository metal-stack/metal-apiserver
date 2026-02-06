package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	valkeygo "github.com/valkey-io/valkey-go"
)

type (
	Queue struct {
		log    *slog.Logger
		client valkeygo.Client
	}
)

func New(log *slog.Logger, client valkeygo.Client) *Queue {
	return &Queue{
		log:    log,
		client: client,
	}
}

// Machine BMC Commands

func (q *Queue) PushMachineCommand(ctx context.Context, partition string, command task.MachineBMCCommandPayload) error {
	return Push(ctx, q.log, q.client, partition, command)
}

func (q *Queue) PushMachineCommandDone(ctx context.Context, commandId string, command task.BMCCommandDonePayload) error {
	return Push(ctx, q.log, q.client, commandId, command)
}

func (q *Queue) WaitMachineCommandDone(ctx context.Context, commandId string) <-chan task.BMCCommandDonePayload {
	return Wait[task.BMCCommandDonePayload](ctx, q.log, q.client, commandId)
}

func (q *Queue) WaitMachineCommand(ctx context.Context, partition string) <-chan task.MachineBMCCommandPayload {
	return Wait[task.MachineBMCCommandPayload](ctx, q.log, q.client, partition)
}

// Machine Allocation

func (q *Queue) PushMachineAllocation(ctx context.Context, machineId string, allocation task.MachineAllocationPayload) error {
	return Push(ctx, q.log, q.client, machineId, allocation)
}

func (q *Queue) WaitMachineAllocation(ctx context.Context, machineId string) <-chan task.MachineAllocationPayload {
	return Wait[task.MachineAllocationPayload](ctx, q.log, q.client, machineId)
}

// Push allows adding a value to the queue which will then be delivered to the waiters.
// messages will only be received by one waiter which waits for the given queueName.
func Push(ctx context.Context, log *slog.Logger, client valkeygo.Client, queueName string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	err = client.Do(ctx, client.B().Rpush().Key(queueName).Element(string(encoded)).Build()).Error()
	if err != nil {
		return fmt.Errorf("push to queue %s with error %w", queueName, err)
	}
	log.Debug("pushed", "queueName", queueName, "value", encoded)
	return nil
}

// Wait receives values from the queue with the given queueName, every value in the queue not already consumed will be returned.
// when the context is done, the channel gets closed.
func Wait[E any](ctx context.Context, log *slog.Logger, client valkeygo.Client, queueName string) <-chan E {
	var (
		resChan = make(chan E)
	)

	go func() {
		defer close(resChan)

		for {
			log.Debug("waiting for queue entries", "queueName", queueName)

			results, err := client.Do(ctx, client.B().Brpop().Key(queueName).Timeout(0).Build()).AsStrSlice()
			if err != nil {
				if errors.Is(err, valkeygo.ErrClosing) {
					log.Error("valkey client is either closed, or valkey is down")
					return
				}
				select {
				case <-ctx.Done():
					log.Debug("context closed")
					return
				default:
					log.Error("error reading from queue", "error", err)
					continue
				}
			}

			log.Debug("wait received", "results", results)
			if len(results) != 2 {
				log.Error("expected exactly 2 elements in results, continue")
				continue
			}

			result := results[1]
			var elem E
			err = json.Unmarshal([]byte(result), &elem)
			if err != nil {
				log.Error("unable to decode result", "error", err)
				continue
			}

			resChan <- elem

		}
	}()

	return resChan
}
