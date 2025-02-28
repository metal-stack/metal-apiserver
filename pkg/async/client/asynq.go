package client

import (
	"encoding/json"
	"log/slog"
	"time"

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
	}
)

func New(log *slog.Logger, redis *redis.Client) *Client {
	client := asynq.NewClientFromRedisClient(redis)

	return &Client{
		log:    log,
		client: client,
	}
}

func (a *Client) Enqueue(task *asynq.Task) (*asynq.TaskInfo, error) {
	a.log.Debug("enqueue", "task", task.Type())
	return a.client.Enqueue(task)
}

//----------------------------------------------
// Write a function NewXXXTask to create a task.
// A task consists of a type and a payload.
//----------------------------------------------

func (a *Client) NewIPDeleteTask(allocationUUID, ip, project string) (*asynq.TaskInfo, error) {
	payload, err := json.Marshal(IPDeletePayload{AllocationUUID: allocationUUID, IP: ip, Project: project})
	if err != nil {
		return nil, err
	}
	// TODO configurable retry and timeout
	task, err := asynq.NewTask(TypeIpDelete, payload, asynq.MaxRetry(5), asynq.Timeout(20*time.Minute)), nil
	if err != nil {
		return nil, err
	}
	return a.Enqueue(task)
}

func (a *Client) NewNetworkDeleteTask(uuid string) (*asynq.TaskInfo, error) {
	payload, err := json.Marshal(NetworkDeletePayload{UUID: uuid})
	if err != nil {
		return nil, err
	}
	// TODO configurable retry and timeout
	task, err := asynq.NewTask(TypeIpDelete, payload, asynq.MaxRetry(5), asynq.Timeout(20*time.Minute)), nil
	if err != nil {
		return nil, err
	}
	return a.Enqueue(task)
}
