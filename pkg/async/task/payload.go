package task

const (
	TypeIpDelete          TaskType = "ip:delete"
	TypeNetworkDelete     TaskType = "network:delete"
	TypeMachineDelete     TaskType = "machine:delete"
	TypeMachineBMCCommand TaskType = "machine:bmc-command"
	TypeMachineAllocation TaskType = "machine:allocation"
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
		// TODO maybe we need the allocated ips during create here as well
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
