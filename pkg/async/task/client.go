package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

const (
	TypeIpDelete          TaskType = "ip:delete"
	TypeNetworkDelete     TaskType = "network:delete"
	TypeMachineDelete     TaskType = "machine:delete"
	TypeMachineBMCCommand TaskType = "machine:bmc-command"
	TypeMachineAllocation TaskType = "machine:allocation"
)

var (
	defaultAsynqRetries = asynq.MaxRetry(50)
	defaultAsynqTimeout = asynq.Timeout(20 * time.Minute)
)

type (
	TaskType string

	IPDeletePayload struct {
		AllocationUUID string `json:"allocation_uuid,omitempty"`
		IP             string `json:"ip,omitempty"`
		Project        string `json:"project,omitempty"`
	}

	NetworkDeletePayload struct {
		UUID string `json:"uuid,omitempty"`
	}

	MachineDeletePayload struct {
		// UUID of the machine which should be deleted (the machine)
		UUID *string `json:"uuid,omitempty"`
		// AllocationUUID of the machine allocation which should be deleted
		AllocationUUID *string `json:"allocation_uuid,omitempty"`
	}

	MachineBMCCommandPayload struct {
		// UUID of the machine where the command should be executed against
		UUID string `json:"uuid,omitempty"`
		// Partition where the machine resides
		Partition string `json:"partition,omitempty"`
		// The actual command
		Command string `json:"command,omitempty"`
		// CommandID identifies this command unique
		CommandID string `json:"command_id"`
	}

	BMCCommandDonePayload struct {
		Error *string `json:"error,omitempty"`
	}

	MachineAllocationPayload struct {
		// UUID of the machine which was allocated and trigger the machine installation
		UUID string `json:"uuid,omitempty"`
	}

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

//----------------------------------------------
// Write a function NewXXXTask to create a task.
// A task consists of a type and a payload.
//----------------------------------------------

func (c *Client) NewTask(payload any, additionalOpts ...asynq.Option) (*asynq.TaskInfo, error) {
	var taskType TaskType

	switch payload.(type) {
	case *IPDeletePayload:
		taskType = TypeIpDelete
	case *NetworkDeletePayload:
		taskType = TypeNetworkDelete
	case *MachineDeletePayload:
		taskType = TypeMachineDelete
	case *MachineBMCCommandPayload:
		taskType = TypeMachineBMCCommand
	default:
		return nil, fmt.Errorf("no task for payload of type %T", payload)
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal task payload: %w", err)
	}

	taskId, err := taskID()
	if err != nil {
		return nil, err
	}

	var (
		opts = append(c.opts, asynq.TaskID(taskId))
		task = asynq.NewTask(string(taskType), encoded, append(opts, additionalOpts...)...)
	)

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
