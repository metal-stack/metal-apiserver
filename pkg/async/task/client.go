package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/redis/go-redis/v9"
)

// A list of task types.
const (
	TypeIpDelete          = "ip:delete"
	TypeNetworkDelete     = "network:delete"
	TypeMachineDelete     = "machine:delete"
	TypeMachineBMCCommand = "machine:bmc-command"
	TypeMachineAllocation = "machine:allocation"
)

var (
	defaultAsynqRetries = asynq.MaxRetry(50)
	defaultAsynqTimeout = asynq.Timeout(20 * time.Minute)
)

type (
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
)

func NewClient(log *slog.Logger, redis *redis.Client, opts ...asynq.Option) *Client {
	client := asynq.NewClientFromRedisClient(redis)

	// Set default opts
	if len(opts) == 0 {
		opts = []asynq.Option{defaultAsynqRetries, defaultAsynqTimeout}
	}

	inspector := asynq.NewInspectorFromRedisClient(redis)

	return &Client{
		log:       log,
		client:    client,
		inspector: inspector,
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

func (c *Client) List(queue *string, count, page *uint32) ([]*asynq.TaskInfo, error) {
	if queue != nil {
		return c.list(*queue, count, page)
	}

	queues, err := c.GetQueues()
	if err != nil {
		return nil, err
	}
	var tasks []*asynq.TaskInfo
	for _, q := range queues {
		ts, err := c.list(q, count, page)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, ts...)
	}
	return tasks, nil
}

func (c *Client) list(queue string, count, page *uint32) ([]*asynq.TaskInfo, error) {
	if count == nil {
		count = pointer.Pointer(uint32(100))
	}
	if page == nil {
		page = pointer.Pointer(uint32(1))
	}

	opts := []asynq.ListOption{asynq.PageSize(int(*count)), asynq.Page(int(*page))}

	var tasks []*asynq.TaskInfo
	active, err := c.inspector.ListActiveTasks(queue, opts...)
	if err != nil {
		return nil, err
	}
	tasks = append(tasks, active...)
	pending, err := c.inspector.ListPendingTasks(queue)
	if err != nil {
		return nil, err
	}
	tasks = append(tasks, pending...)
	scheduled, err := c.inspector.ListScheduledTasks(queue)
	if err != nil {
		return nil, err
	}
	tasks = append(tasks, scheduled...)
	retry, err := c.inspector.ListRetryTasks(queue)
	if err != nil {
		return nil, err
	}
	tasks = append(tasks, retry...)
	archived, err := c.inspector.ListArchivedTasks(queue)
	if err != nil {
		return nil, err
	}
	tasks = append(tasks, archived...)
	completed, err := c.inspector.ListCompletedTasks(queue)
	if err != nil {
		return nil, err
	}
	tasks = append(tasks, completed...)
	return tasks, nil
}

//----------------------------------------------
// Write a function NewXXXTask to create a task.
// A task consists of a type and a payload.
//----------------------------------------------

func (c *Client) NewIPDeleteTask(allocationUUID, ip, project string) (*asynq.TaskInfo, error) {
	payload, err := json.Marshal(IPDeletePayload{AllocationUUID: allocationUUID, IP: ip, Project: project})
	if err != nil {
		return nil, fmt.Errorf("unable to marshal ip delete payload:%w", err)
	}

	taskId, err := taskID()
	if err != nil {
		return nil, err
	}
	opts := append(c.opts, asynq.TaskID(taskId))

	task := asynq.NewTask(TypeIpDelete, payload, opts...)
	taskInfo, err := c.client.Enqueue(task)
	if err != nil {
		return nil, fmt.Errorf("unable to enqueue ip delete task:%w", err)
	}
	return taskInfo, nil
}

func (c *Client) NewNetworkDeleteTask(uuid string) (*asynq.TaskInfo, error) {
	payload, err := json.Marshal(NetworkDeletePayload{UUID: uuid})
	if err != nil {
		return nil, fmt.Errorf("unable to marshal network delete payload:%w", err)
	}

	taskId, err := taskID()
	if err != nil {
		return nil, err
	}
	opts := append(c.opts, asynq.TaskID(taskId))

	task := asynq.NewTask(TypeNetworkDelete, payload, opts...)
	taskInfo, err := c.client.Enqueue(task)
	if err != nil {
		return nil, fmt.Errorf("unable to enqueue network delete task:%w", err)
	}
	return taskInfo, nil
}

func (c *Client) NewMachineDeleteTask(uuid, allocationUUID *string) (*asynq.TaskInfo, error) {
	payload, err := json.Marshal(MachineDeletePayload{UUID: uuid, AllocationUUID: allocationUUID})
	if err != nil {
		return nil, fmt.Errorf("unable to marshal machine delete payload:%w", err)
	}

	taskId, err := taskID()
	if err != nil {
		return nil, err
	}
	opts := append(c.opts, asynq.TaskID(taskId))

	task := asynq.NewTask(TypeMachineDelete, payload, opts...)
	taskInfo, err := c.client.Enqueue(task)
	if err != nil {
		return nil, fmt.Errorf("unable to enqueue machine delete task:%w", err)
	}
	return taskInfo, nil
}

func (c *Client) NewMachineBMCCommandTask(machineUuid, partition, command string) (*asynq.TaskInfo, error) {
	taskId, err := taskID()
	if err != nil {
		return nil, err
	}

	commandId := machineUuid + ":machine-bmc-command:" + command
	payload, err := json.Marshal(MachineBMCCommandPayload{
		UUID:      machineUuid,
		Partition: partition,
		Command:   command,
		CommandID: commandId,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to marshal machine bmc command payload:%w", err)
	}

	task := asynq.NewTask(
		TypeMachineBMCCommand,
		payload,
		asynq.TaskID(taskId),
		asynq.Timeout(time.Minute),
		asynq.MaxRetry(0),
		asynq.Retention(30*24*time.Hour), // Only with retention a task will be stored in completed tasks
	)
	taskInfo, err := c.client.Enqueue(task, asynq.Unique(time.Minute))

	switch {
	case errors.Is(err, asynq.ErrDuplicateTask):
		return nil, fmt.Errorf("machine bmc command is still in progress for machine %s %w", machineUuid, err)
	case err != nil:
		return nil, fmt.Errorf("unable to enqueue machine bmc command task:%w", err)
	}
	return taskInfo, nil
}

// taskID generate a random taskID to ensure unique execution
// see: https://github.com/hibiken/asynq/wiki/Unique-Tasks
func taskID() (string, error) {
	taskId, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("unable to generate a unique task id:%w", err)
	}
	return taskId.String(), nil
}
