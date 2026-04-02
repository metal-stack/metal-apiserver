package task

import apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"

const (
	TypeIpDelete          TaskType = "ip:delete"
	TypeNetworkDelete     TaskType = "network:delete"
	TypeMachineDelete     TaskType = "machine:delete"
	TypeMachineBMCCommand TaskType = "machine:bmc-command"
	TypeMachineAllocation TaskType = "machine:allocation"
	TypeMachineCreate     TaskType = "machine:create"
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

	// MachineCreatePayload is used to create a transaction to allocate a machine
	MachineCreatePayload struct {
		// UUID of this create request to get notified when this machine create task was finished
		UUID string `json:"uuid,omitempty"`
		// Request to create a machine
		Request *apiv2.MachineServiceCreateRequest `json:"request,omitempty"`
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

func (p *MachineCreatePayload) Type() TaskType {
	return TypeMachineCreate
}
