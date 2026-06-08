package task

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const (
	TypeIpDelete          TaskType = "ip:delete"
	TypeNetworkDelete     TaskType = "network:delete"
	TypeMachineDelete     TaskType = "machine:delete"
	TypeMachineBMCCommand TaskType = "machine:bmc-command"
)

type (
	TaskType string

	TaskPayload interface {
		Type() TaskType
	}

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
		UUID string `json:"uuid"`
		// AllocationUUID of the machine allocation which should be deleted
		AllocationUUID string `json:"allocation_uuid"`
		// MachineIpAllocationUUIDs are the machine ips that belong to machine allocation at the point of deletion
		MachineIpAllocationUUIDs []string `json:"machine_ip_allocation_uuids,omitempty"`
		// Project is the project on which the machine is allocated
		Project string `json:"project"`
		// Partition is the partition in which the machine is located
		Partition string `json:"partition"`
		// RackID is the rack id in which the machine is located
		RackID string `json:"rack_id"`
		// HeadscaleNodeID is the vpn node id that the firewall was connected with
		HeadscaleNodeID *uint64 `json:"headscale_node_id"`
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
)

func (p *IPDeletePayload) Type() TaskType {
	return TypeIpDelete
}

func (p *NetworkDeletePayload) Type() TaskType {
	return TypeNetworkDelete
}

func (p *MachineDeletePayload) Type() TaskType {
	return TypeMachineDelete
}

func (p *MachineBMCCommandPayload) Type() TaskType {
	return TypeMachineBMCCommand
}

// EncodePayload can be used to encode a task payload using json marshal.
func EncodePayload(payload TaskPayload) ([]byte, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal task payload: %w", err)
	}

	return encoded, nil
}

// DecodePayload can be used to decode a task payload using json unmarshal.
// If an error occurs while decoding, an asynq.SkipRetry error gets wrapped to prevent retry of the task.
func DecodePayload[t TaskPayload](data []byte) (t, error) {
	var payload t

	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, fmt.Errorf("unable to unmarshal task payload, %w: %w", asynq.SkipRetry, err)
	}

	return payload, nil
}
