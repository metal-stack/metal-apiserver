package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

const (
	defaultTaskRetention         = 14 * 24 * time.Hour // only with retention a task will be stored after task failure
	defaultTaskWatchTimeout      = 10 * time.Second
	defaultTaskWatchPollInterval = 100 * time.Millisecond
)

var (
	defaultAsynqRetries = asynq.MaxRetry(50)
	defaultAsynqTimeout = asynq.Timeout(20 * time.Minute)
)

type (
	Client struct {
		log       *slog.Logger
		client    *asynq.Client
		inspector *asynq.Inspector
		opts      []asynq.Option
	}

	taskListFn func(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
)

func NewClient(log *slog.Logger, redis *redis.Client, opts ...asynq.Option) *Client {
	client := asynq.NewClientFromRedisClient(redis)

	// Set default opts
	if len(opts) == 0 {
		opts = []asynq.Option{defaultAsynqRetries, defaultAsynqTimeout}
	}

	return &Client{
		log:       log,
		client:    client,
		inspector: asynq.NewInspectorFromRedisClient(redis),
		opts:      opts,
	}
}

func (c *Client) GetQueues() ([]string, error) {
	return c.inspector.Queues()
}

func (c *Client) GetTaskInfo(queue, id string) (*asynq.TaskInfo, error) {
	return c.inspector.GetTaskInfo(queue, id)
}

func (c *Client) DeleteTask(queue, id string) error {
	return c.inspector.DeleteTask(queue, id)
}

func (c *Client) Ping() error {
	return c.client.Ping()
}

// WatchConfig can be used to overwrite some watch configuration defaults.
// Can be provided optionally by callers.
type WatchConfig struct {
	// Timeout specifies when the watch cancels in case no state is getting reached.
	// Set to 0 to only depend on parent context cancellation.
	Timeout *time.Duration
	// Interval specifies how often the state is checked in the backend.
	Interval *time.Duration
}

func (c *Client) WatchForTaskCompletion(ctx context.Context, cfg *WatchConfig, queue, id string) (info *asynq.TaskInfo, err error) {
	var (
		timeout  = defaultTaskWatchTimeout
		interval = defaultTaskWatchPollInterval

		// a task is put to archive if it failed and will not be retried, to completed if it succeeds successfully
		finalStates = []asynq.TaskState{asynq.TaskStateArchived, asynq.TaskStateCompleted}

		// this is just for nicer error messages
		finalStatesString = func() string {
			var res []string
			for _, state := range finalStates {
				res = append(res, state.String())
			}
			return strings.Join(res, ",")
		}()
	)

	if cfg != nil {
		switch {
		case cfg.Timeout != nil:
			timeout = *cfg.Timeout
		case cfg.Interval != nil:
			interval = *cfg.Interval
		}
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var err error
			info, err = c.GetTaskInfo(queue, id)
			if err != nil {
				return nil, err
			}

			if slices.Contains(finalStates, info.State) {
				return info, nil
			}

			c.log.Debug("watched task state not yet reached", "task-id", info.ID, "task-type", info.Type, "got", info.State.String(), "want", finalStatesString, "last-err", info.LastErr)
			continue

		case <-ctx.Done():
			if info != nil {
				return nil, fmt.Errorf("context cancelled, task %q of type %q did not reach one of expected states %q (had %q), last error: %s", info.ID, info.Type, finalStatesString, info.State.String(), info.LastErr)
			}

			return nil, fmt.Errorf("context cancelled before the task %q of type %q was even received once", info.ID, info.Type)
		}
	}
}

func (c *Client) List(queue *string) ([]*asynq.TaskInfo, error) {
	if queue != nil {
		return c.listQueueTasks(*queue)
	}

	queues, err := c.GetQueues()
	if err != nil {
		return nil, err
	}

	var tasks []*asynq.TaskInfo

	for _, q := range queues {
		ts, err := c.listQueueTasks(q)
		if err != nil {
			return nil, err
		}

		tasks = append(tasks, ts...)
	}

	return tasks, nil
}

func (c *Client) listQueueTasks(queue string) ([]*asynq.TaskInfo, error) {
	var all []*asynq.TaskInfo

	for _, listFn := range []taskListFn{
		c.inspector.ListActiveTasks,
		c.inspector.ListPendingTasks,
		c.inspector.ListRetryTasks,
		c.inspector.ListArchivedTasks,
		c.inspector.ListCompletedTasks,
	} {
		res, err := listAll(queue, listFn)
		if err != nil {
			return nil, err
		}

		all = append(all, res...)
	}

	return all, nil
}

func listAll(queue string, callFn taskListFn) ([]*asynq.TaskInfo, error) {
	var (
		size = 1000
		page = 0
		all  []*asynq.TaskInfo
	)

	for {
		res, err := callFn(queue, asynq.PageSize(size), asynq.Page(page))
		if err != nil {
			return nil, fmt.Errorf("unable to list tasks in queue %q: %w", queue, err)
		}

		all = append(all, res...)
		page++

		if len(res) >= size {
			continue
		}

		return all, nil
	}
}

func (c *Client) NewTask(payload TaskPayload, additionalOpts ...asynq.Option) (*asynq.TaskInfo, error) {
	encoded, err := EncodePayload(payload)
	if err != nil {
		return nil, err
	}

	taskId, err := taskID()
	if err != nil {
		return nil, err
	}

	var (
		opts = append(c.opts, asynq.TaskID(taskId))
	)

	if !slices.ContainsFunc(additionalOpts, func(opt asynq.Option) bool {
		return opt.Type() == asynq.RetentionOpt
	}) {
		opts = append(opts, asynq.Retention(defaultTaskRetention))
	}

	task := asynq.NewTask(string(payload.Type()), encoded, append(opts, additionalOpts...)...)

	taskInfo, err := c.client.Enqueue(task)

	switch {
	case errors.Is(err, asynq.ErrDuplicateTask):
		return nil, fmt.Errorf("a task is already enqueued: %w", err)
	case err != nil:
		return nil, fmt.Errorf("unable to enqueue task: %w", err)
	}

	return taskInfo, nil
}

// taskID generate a random taskID to ensure unique execution
// see: https://github.com/hibiken/asynq/wiki/Unique-Tasks
func taskID() (string, error) {
	taskId, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("unable to generate a unique task id: %w", err)
	}

	return taskId.String(), nil
}
