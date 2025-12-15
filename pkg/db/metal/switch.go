package metal

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

type (
	Switch struct {
		Base
		Rack               string            `rethinkdb:"rackid"`
		Partition          string            `rethinkdb:"partitionid"`
		ReplaceMode        SwitchReplaceMode `rethinkdb:"mode"`
		ManagementIP       string            `rethinkdb:"management_ip"`
		ManagementUser     string            `rethinkdb:"management_user"`
		ConsoleCommand     string            `rethinkdb:"console_command"`
		OS                 *SwitchOS         `rethinkdb:"os"`
		Nics               Nics              `rethinkdb:"network_interfaces"`
		MachineConnections ConnectionMap     `rethinkdb:"machineconnections"`
	}
	Switches []Switch

	SwitchStatus struct {
		Base
		LastSync      *SwitchSync `rethinkdb:"last_sync"`
		LastSyncError *SwitchSync `rethinkdb:"last_sync_error"`
	}

	SwitchBGPPortState struct {
		// FIXME add rethinkdb annotations, check against existing database entries
		Neighbor              string
		PeerGroup             string
		VrfName               string
		BgpState              BGPState
		BgpTimerUpEstablished uint64
		SentPrefixCounter     uint64
		AcceptedPrefixCounter uint64
	}

	SwitchSync struct {
		Time     time.Time     `rethinkdb:"time"`
		Duration time.Duration `rethinkdb:"duration"`
		Error    *string       `rethinkdb:"error"`
	}

	Connection struct {
		Nic       Nic    `rethinkdb:"nic"`
		MachineID string `rethinkdb:"machineid"`
	}
	Connections []Connection

	// ConnectionMap maps machine ids to connections
	ConnectionMap map[string]Connections

	SwitchOS struct {
		Vendor           SwitchOSVendor `rethinkdb:"vendor"`
		Version          string         `rethinkdb:"version"`
		MetalCoreVersion string         `rethinkdb:"metal_core_version"`
	}

	SwitchReplaceMode string
	SwitchOSVendor    string
)

const (
	SwitchReplaceModeReplace     = SwitchReplaceMode("replace")
	SwitchReplaceModeOperational = SwitchReplaceMode("operational")

	SwitchOSVendorCumulus = SwitchOSVendor("Cumulus")
	SwitchOSVendorSonic   = SwitchOSVendor("SONiC")
)

func ToReplaceMode(mode apiv2.SwitchReplaceMode) (SwitchReplaceMode, error) {
	strVal, err := enum.GetStringValue(mode)
	if err != nil {
		return SwitchReplaceMode(""), err
	}
	return SwitchReplaceMode(*strVal), nil
}

func FromReplaceMode(mode SwitchReplaceMode) (apiv2.SwitchReplaceMode, error) {
	apiv2ReplaceMode, err := enum.GetEnum[apiv2.SwitchReplaceMode](string(mode))
	if err != nil {
		return apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_UNSPECIFIED, fmt.Errorf("switch replace mode %q is invalid", mode)
	}
	return apiv2ReplaceMode, nil
}

func ToSwitchOSVendor(vendor apiv2.SwitchOSVendor) (SwitchOSVendor, error) {
	strVal, err := enum.GetStringValue(vendor)
	if err != nil {
		return SwitchOSVendor(""), err
	}
	return SwitchOSVendor(*strVal), nil
}

func FromSwitchOSVendor(vendor SwitchOSVendor) (apiv2.SwitchOSVendor, error) {
	apiv2Vendor, err := enum.GetEnum[apiv2.SwitchOSVendor](string(vendor))
	if err != nil {
		return apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_UNSPECIFIED, fmt.Errorf("switch os vendor %q is invalid", vendor)
	}
	return apiv2Vendor, nil
}

func ToSwitchPortStatus(status apiv2.SwitchPortStatus) (SwitchPortStatus, error) {
	strVal, err := enum.GetStringValue(status)
	if err != nil {
		return SwitchPortStatus(""), err
	}
	return SwitchPortStatus(strings.ToUpper(*strVal)), nil
}

func FromSwitchPortStatus(status *SwitchPortStatus) (apiv2.SwitchPortStatus, error) {
	if status == nil {
		return apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNSPECIFIED, nil
	}

	apiv2Status, err := enum.GetEnum[apiv2.SwitchPortStatus](strings.ToLower(string(*status)))
	if err != nil {
		return apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNSPECIFIED, fmt.Errorf("switch port status %q is invalid", *status)
	}
	return apiv2Status, nil
}

func ToBGPState(state apiv2.BGPState) (BGPState, error) {
	strVal, err := enum.GetStringValue(state)
	if err != nil {
		return BGPState(""), err
	}
	return BGPState(*strVal), nil
}

func FromBGPState(state BGPState) (apiv2.BGPState, error) {
	apiv2State, err := enum.GetEnum[apiv2.BGPState](string(state))
	if err != nil {
		return apiv2.BGPState_BGP_STATE_UNSPECIFIED, fmt.Errorf("bgp state %q is invalid", state)
	}
	return apiv2State, nil
}

func (c ConnectionMap) ByNicName() (map[string]Connection, error) {
	res := make(map[string]Connection)
	for _, cons := range c {
		for _, con := range cons {
			if _, has := res[con.Nic.Name]; has {
				return nil, fmt.Errorf("switch port %s is connected to more than one machine", con.Nic.Name)
			}
			res[con.Nic.Name] = con
		}
	}
	return res, nil
}

func (s *Switch) ConnectMachine(machineID string, machineNics Nics) (int, error) {
	_, connectionExists := s.MachineConnections[machineID]
	physicalConnections := s.getPhysicalMachineConnections(machineID, machineNics)

	if len(physicalConnections) < 1 {
		if connectionExists {
			return 0, fmt.Errorf("machine connection between machine %s and switch %s exists in the database but not physically; if you are attempting migrate the machine from one rack to another delete it first", machineID, s.ID)
		}
		return 0, nil
	}

	delete(s.MachineConnections, machineID)
	s.MachineConnections[machineID] = append(s.MachineConnections[machineID], physicalConnections...)
	return len(physicalConnections), nil
}

// TranslateNicMap creates a NicMap where the keys are translated to the naming convention of the target OS
//
// example mapping from cumulus to sonic for one single port:
//
//	map[string]Nic {
//		"swp1s1": Nic{
//			Name: "Ethernet1",
//			MacAddress: ""
//		}
//	}
func (s *Switch) TranslateNicMap(targetOS SwitchOSVendor) (NicMap, error) {
	nicMap := s.Nics.MapByName()
	translatedNicMap := make(NicMap)

	if s.OS.Vendor == targetOS {
		return nicMap, nil
	}

	ports := make([]string, 0)
	for name := range nicMap {
		ports = append(ports, name)
	}

	lines, err := getLinesFromPortNames(ports, s.OS.Vendor)
	if err != nil {
		return nil, err
	}

	for _, p := range ports {
		targetPort, err := mapPortName(p, s.OS.Vendor, targetOS, lines)
		if err != nil {
			return nil, err
		}

		nic, ok := nicMap[p]
		if !ok {
			return nil, fmt.Errorf("an unknown error occurred during port name translation")
		}
		translatedNicMap[targetPort] = nic
	}

	return translatedNicMap, nil
}

// MapPortNames creates a dictionary that maps the naming convention of this switch's OS to that of the target OS
func (s *Switch) MapPortNames(targetOS SwitchOSVendor) (map[string]string, error) {
	nics := s.Nics.MapByName()
	portNamesMap := make(map[string]string, len(s.Nics))

	ports := make([]string, 0)
	for name := range nics {
		ports = append(ports, name)
	}

	lines, err := getLinesFromPortNames(ports, s.OS.Vendor)
	if err != nil {
		return nil, err
	}

	for _, p := range ports {
		targetPort, err := mapPortName(p, s.OS.Vendor, targetOS, lines)
		if err != nil {
			return nil, err
		}
		portNamesMap[p] = targetPort
	}

	return portNamesMap, nil
}

func mapPortName(port string, sourceOS, targetOS SwitchOSVendor, allLines []int) (string, error) {
	line, err := portNameToLine(port, sourceOS)
	if err != nil {
		return "", fmt.Errorf("unable to get line number from port name, %w", err)
	}

	switch targetOS {
	case SwitchOSVendorSonic:
		return sonicPortByLineNumber(line), nil
	case SwitchOSVendorCumulus:
		return cumulusPortByLineNumber(line, allLines), nil
	default:
		return "", fmt.Errorf("unknown target switch os %s", targetOS)
	}
}

func getLinesFromPortNames(ports []string, os SwitchOSVendor) ([]int, error) {
	lines := make([]int, 0)
	for _, p := range ports {
		l, err := portNameToLine(p, os)
		if err != nil {
			return nil, fmt.Errorf("unable to get line number from port name, %w", err)
		}

		lines = append(lines, l)
	}

	return lines, nil
}

func portNameToLine(port string, os SwitchOSVendor) (int, error) {
	switch os {
	case SwitchOSVendorSonic:
		return sonicPortNameToLine(port)
	case SwitchOSVendorCumulus:
		return cumulusPortNameToLine(port)
	default:
		return 0, fmt.Errorf("unknown switch os %s", os)
	}

}

func sonicPortNameToLine(port string) (int, error) {
	// to prevent accidentally parsing a substring to a negative number
	if strings.Contains(port, "-") {
		return 0, fmt.Errorf("invalid token '-' in port name %s", port)
	}

	prefix, lineString, found := strings.Cut(port, "Ethernet")
	if !found {
		return 0, fmt.Errorf("invalid port name %s, expected to find prefix 'Ethernet'", port)
	}

	if prefix != "" {
		return 0, fmt.Errorf("invalid port name %s, port name is expected to start with 'Ethernet'", port)
	}

	line, err := strconv.Atoi(lineString)
	if err != nil {
		return 0, fmt.Errorf("unable to convert port name to line number: %w", err)
	}

	return line, nil
}

func cumulusPortNameToLine(port string) (int, error) {
	// to prevent accidentally parsing a substring to a negative number
	if strings.Contains(port, "-") {
		return 0, fmt.Errorf("invalid token '-' in port name %s", port)
	}

	prefix, suffix, found := strings.Cut(port, "swp")
	if !found {
		return 0, fmt.Errorf("invalid port name %s, expected to find prefix 'swp'", port)
	}

	if prefix != "" {
		return 0, fmt.Errorf("invalid port name %s, port name is expected to start with 'swp'", port)
	}

	var line int

	countString, indexString, found := strings.Cut(suffix, "s")
	if !found {
		count, err := strconv.Atoi(suffix)
		if err != nil {
			return 0, fmt.Errorf("unable to convert port name to line number: %w", err)
		}
		if count <= 0 {
			return 0, fmt.Errorf("invalid port name %s would map to negative number", port)
		}
		line = (count - 1) * 4
	} else {
		count, err := strconv.Atoi(countString)
		if err != nil {
			return 0, fmt.Errorf("unable to convert port name to line number: %w", err)
		}
		if count <= 0 {
			return 0, fmt.Errorf("invalid port name %s would map to negative number", port)
		}

		index, err := strconv.Atoi(indexString)
		if err != nil {
			return 0, fmt.Errorf("unable to convert port name to line number: %w", err)
		}
		line = (count-1)*4 + index
	}

	return line, nil
}

func sonicPortByLineNumber(line int) string {
	return fmt.Sprintf("Ethernet%d", line)
}

func cumulusPortByLineNumber(line int, allLines []int) string {
	if line%4 > 0 {
		return fmt.Sprintf("swp%ds%d", line/4+1, line%4)
	}

	for _, l := range allLines {
		if l == line {
			continue
		}
		if l/4 == line/4 {
			return fmt.Sprintf("swp%ds%d", line/4+1, line%4)
		}
	}

	return fmt.Sprintf("swp%d", line/4+1)
}

func (s *Switch) getPhysicalMachineConnections(machineID string, machineNics Nics) Connections {
	connections := make(Connections, 0)
	for _, machineNic := range machineNics {
		neighMap := machineNic.Neighbors.FilterByHostname(s.ID).MapByIdentifier()

		for _, switchNic := range s.Nics {
			if _, has := neighMap[switchNic.Identifier]; has {
				connections = append(connections, Connection{
					Nic:       switchNic,
					MachineID: machineID,
				})
			}
		}
	}
	return connections
}
