package client

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
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
		// Timestamp when this command was issued.
		// Older machinecommands are dropped silently
		// TODO define max command age
		IssuedAt time.Time `json:"issued_at"`
		// CommandID identifies this command unique
		CommandID string `json:"command_id"`
	}

	BMCCommandDonePayload struct {
		Error *string `json:"error,omitempty"`
	}

	Client struct {
		client *asynq.Client
		log    *slog.Logger
		opts   []asynq.Option
	}
)

func New(log *slog.Logger, redis *redis.Client, opts ...asynq.Option) *Client {
	client := asynq.NewClientFromRedisClient(redis)

	// Set default opts
	if len(opts) == 0 {
		opts = append([]asynq.Option{defaultAsynqRetries, defaultAsynqTimeout}, opts...)
	}

	return &Client{
		log:    log,
		client: client,
		opts:   opts,
	}
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
	opts := append(c.opts, asynq.TaskID(taskId))

	task := asynq.NewTask(TypeMachineDelete, payload, opts...)
	taskInfo, err := c.client.Enqueue(task)
	if err != nil {
		return nil, fmt.Errorf("unable to enqueue machine delete task:%w", err)
	}
	return taskInfo, nil
}

func (c *Client) NewMachineBMCCommandTask(uuid, partition, command string) (*asynq.TaskInfo, error) {
	taskId := uuid + ":machine-bmc-command"

	payload, err := json.Marshal(MachineBMCCommandPayload{
		UUID:      uuid,
		Partition: partition,
		Command:   command,
		IssuedAt:  time.Now(),
		CommandID: taskId,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to marshal machine bmc command payload:%w", err)
	}

	task := asynq.NewTask(TypeMachineBMCCommand, payload, asynq.TaskID(taskId), asynq.Timeout(time.Minute))
	taskInfo, err := c.client.Enqueue(task)
	if err != nil {
		return nil, fmt.Errorf("unable to enqueue machine bmc command task:%w", err)
	}
	return taskInfo, nil
}

// addTaskID generate a random taskID to ensure unique execution
// see: https://github.com/hibiken/asynq/wiki/Unique-Tasks
func taskID() (string, error) {
	taskId, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("unable to generate a unique task id:%w", err)
	}
	return taskId.String(), nil
}
