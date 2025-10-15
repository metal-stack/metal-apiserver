package metal

type Switch struct {
	Base
	RackID             string            `rethinkdb:"rackid"`
	Partition          string            `rethinkdb:"partitionid"`
	ReplaceMode        SwitchReplaceMode `rethinkdb:"mode"`
	ManagementIP       string            `rethinkdb:"management_ip"`
	ManagementUser     string            `rethinkdb:"management_user"`
	ConsoleCommand     string            `rethinkdb:"console_command"`
	OS                 SwitchOS          `rethinkdb:"os"`
	Nics               Nics              `rethinkdb:"network_interfaces"`
	MachineConnections ConnectionMap     `rethinkdb:"machineconnections"`
}

type Switches []Switch

type Connection struct {
	Nic       Nic    `rethinkdb:"nic"`
	MachineID string `rethinkdb:"machineid"`
}

type Connections []Connection

// ConnectionMap maps machine ids to connections
type ConnectionMap map[string]Connections

type SwitchOS struct {
	Vendor           SwitchOSVendor `rethinkdb:"vendor"`
	Version          string         `rethinkdb:"version"`
	MetalCoreVersion string         `rethinkdb:"metal_core_version"`
}

type SwitchReplaceMode string
type SwitchOSVendor string

const (
	SwitchReplaceModeReplace     = SwitchReplaceMode("replace")
	SwitchReplaceModeOperational = SwitchReplaceMode("operational")

	SwitchOSVendorCumulus = SwitchOSVendor("Cumulus")
	SwitchOSVendorSonic   = SwitchOSVendor("SONiC")
)
