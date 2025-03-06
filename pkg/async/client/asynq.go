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
	TypeIpDelete      = "ip:delete"
	TypeNetworkDelete = "network:delete"
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

	Client struct {
		client *asynq.Client
		log    *slog.Logger
		opts   []asynq.Option
	}
)

func New(log *slog.Logger, redis *redis.Client) *Client {
	client := asynq.NewClientFromRedisClient(redis)

	opts := []asynq.Option{
		asynq.MaxRetry(5),
		asynq.Timeout(20 * time.Minute),
	}
	return &Client{
		log:    log,
		client: client,
		opts:   opts,
	}
}

func (c *Client) enqueue(task *asynq.Task) (*asynq.TaskInfo, error) {
	c.log.Debug("enqueue", "task", task.Type())
	return c.client.Enqueue(task)
}

//----------------------------------------------
// Write a function NewXXXTask to create a task.
// A task consists of a type and a payload.
//----------------------------------------------

func (c *Client) NewIPDeleteTask(allocationUUID, ip, project string) (*asynq.TaskInfo, error) {
	payload, err := json.Marshal(IPDeletePayload{AllocationUUID: allocationUUID, IP: ip, Project: project})
	if err != nil {
		return nil, err
	}

	err = c.addTaskID()
	if err != nil {
		return nil, err
	}

	// TODO configurable retry and timeout
	task, err := asynq.NewTask(TypeIpDelete, payload, c.opts...), nil
	if err != nil {
		return nil, err
	}
	return c.enqueue(task)
}

func (c *Client) NewNetworkDeleteTask(uuid string) (*asynq.TaskInfo, error) {
	payload, err := json.Marshal(NetworkDeletePayload{UUID: uuid})
	if err != nil {
		return nil, err
	}

	err = c.addTaskID()
	if err != nil {
		return nil, err
	}

	// TODO configurable retry and timeout
	task, err := asynq.NewTask(TypeIpDelete, payload, c.opts...), nil
	if err != nil {
		return nil, err
	}
	return c.enqueue(task)
}

// addTaskID generate a random taskID to ensure unique execution
// see: https://github.com/hibiken/asynq/wiki/Unique-Tasks
func (c *Client) addTaskID() error {
	taskId, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("unable to generate a unique task id:%w", err)
	}
	c.opts = append(c.opts, asynq.TaskID(taskId.String()))
	return nil
}
