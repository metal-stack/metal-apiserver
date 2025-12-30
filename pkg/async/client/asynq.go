package client

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
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
		IssuedAt time.Time `json:"issued_at,omitempty"`
	}

	MachineAllocationPayload struct {
		// UUID of the machine which was allocated and trigger the machine installation
		UUID string `json:"uuid,omitempty"`
	}

	Client struct {
		client *asynq.Client
		log    *slog.Logger
		opts   []asynq.Option
		// Mutex to guard access to the maps below
		mu sync.RWMutex
		// A map of machine bmc command channel per partition
		machineBMCCommandChannels map[string]chan *MachineBMCCommandPayload
		// A counter per partition how many receivers are waiting for machine bmc commands
		machineBMCCommandReceivers map[string]int
		// A map if machine uuids to channel
		machineAllocationChannels map[string]chan *MachineAllocationPayload
	}
)

func New(log *slog.Logger, redis *redis.Client, opts ...asynq.Option) *Client {
	client := asynq.NewClientFromRedisClient(redis)

	// Set default opts
	if len(opts) == 0 {
		opts = append([]asynq.Option{defaultAsynqRetries, defaultAsynqTimeout}, opts...)
	}

	var (
		machineBMCCommandChannels  = make(map[string]chan *MachineBMCCommandPayload)
		machineBMCCommandReceivers = make(map[string]int)
		machineAllocationChannels  = make(map[string]chan *MachineAllocationPayload)
	)

	return &Client{
		log:                        log,
		client:                     client,
		opts:                       opts,
		mu:                         sync.RWMutex{},
		machineBMCCommandChannels:  machineBMCCommandChannels,
		machineBMCCommandReceivers: machineBMCCommandReceivers,
		machineAllocationChannels:  machineAllocationChannels,
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

	task := asynq.NewTask(TypeNetworkDelete, payload, c.opts...)
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

	err = c.addTaskID()
	if err != nil {
		return nil, err
	}

	task := asynq.NewTask(TypeMachineDelete, payload, c.opts...)
	taskInfo, err := c.client.Enqueue(task)
	if err != nil {
		return nil, fmt.Errorf("unable to enqueue machine delete task:%w", err)
	}
	return taskInfo, nil
}

func (c *Client) MachineBMCCommandsChannel(partition string) chan *MachineBMCCommandPayload {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.machineBMCCommandChannels[partition]; !ok {
		c.machineBMCCommandChannels[partition] = make(chan *MachineBMCCommandPayload, 100) // TODO how many commands should be in the buffer
	}
	if _, ok := c.machineBMCCommandReceivers[partition]; !ok {
		c.machineBMCCommandReceivers[partition] = 0
	}
	return c.machineBMCCommandChannels[partition]
}

func (c *Client) MachineBMCCommandBeginReceive(partition string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.machineBMCCommandReceivers[partition]; !ok {
		c.machineBMCCommandReceivers[partition] = 0
	}
	c.machineBMCCommandReceivers[partition]++
}

func (c *Client) MachineBMCCommandEndReceive(partition string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.machineBMCCommandReceivers[partition]; !ok {
		c.machineBMCCommandReceivers[partition] = 0
	}
	if c.machineBMCCommandReceivers[partition] == 0 {
		return
	}
	c.machineBMCCommandReceivers[partition]--
}

func (c *Client) MachineBMCCommandHasReceiver(partition string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log.Debug("machine bmc command receivers", "receivers", c.machineBMCCommandReceivers)
	receivers, ok := c.machineBMCCommandReceivers[partition]
	if !ok || receivers == 0 {
		return false
	}
	return true
}

func (c *Client) NewMachineBMCCommandTask(uuid, partition, command string) (*asynq.TaskInfo, error) {
	payload, err := json.Marshal(MachineBMCCommandPayload{
		UUID:      uuid,
		Partition: partition,
		Command:   command,
		IssuedAt:  time.Now(),
	})
	if err != nil {
		return nil, fmt.Errorf("unable to marshal machine bmc command payload:%w", err)
	}

	err = c.addTaskID()
	if err != nil {
		return nil, err
	}

	task := asynq.NewTask(TypeMachineBMCCommand, payload, c.opts...)
	taskInfo, err := c.client.Enqueue(task)
	if err != nil {
		return nil, fmt.Errorf("unable to enqueue machine bmc command task:%w", err)
	}
	return taskInfo, nil
}

func (c *Client) MachineAllocationChannel(uuid string) chan *MachineAllocationPayload {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.machineAllocationChannels[uuid]; !ok {
		c.machineAllocationChannels[uuid] = make(chan *MachineAllocationPayload, 100) // TODO how many commands should be in the buffer
	}
	return c.machineAllocationChannels[uuid]
}

func (c *Client) NewMachineAllocationTask(uuid string) (*asynq.TaskInfo, error) {
	payload, err := json.Marshal(MachineAllocationPayload{
		UUID: uuid,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to marshal machine allocation payload:%w", err)
	}

	err = c.addTaskID()
	if err != nil {
		return nil, err
	}

	task := asynq.NewTask(TypeMachineAllocation, payload, c.opts...)
	taskInfo, err := c.client.Enqueue(task)
	if err != nil {
		return nil, fmt.Errorf("unable to enqueue machine allocation task:%w", err)
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
