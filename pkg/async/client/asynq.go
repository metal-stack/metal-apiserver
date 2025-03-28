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

var (
	defaultAsynqRetries = asynq.MaxRetry(5)
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

	err = c.addTaskID()
	if err != nil {
		return nil, err
	}

	task := asynq.NewTask(TypeIpDelete, payload, c.opts...)
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

	err = c.addTaskID()
	if err != nil {
		return nil, err
	}

	task := asynq.NewTask(TypeIpDelete, payload, c.opts...)
	taskInfo, err := c.client.Enqueue(task)
	if err != nil {
		return nil, fmt.Errorf("unable to enqueue network delete task:%w", err)
	}
	return taskInfo, nil
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
