package metal

import (
	"fmt"
	"net"
	"time"
)

// GetID returns the ID of the entity
func (m *Machine) GetID() string {
	return m.ID
}

// SetID sets the ID of the entity
func (m *Machine) SetID(id string) {
	m.ID = id
}

// GetChanged returns the last changed timestamp of the entity
func (m *Machine) GetChanged() time.Time {
	return m.Changed
}

// SetChanged sets the last changed timestamp of the entity
func (m *Machine) SetChanged(changed time.Time) {
	m.Changed = changed
}

// GetCreated returns the creation timestamp of the entity
func (m *Machine) GetCreated() time.Time {
	return m.Created
}

// SetCreated sets the creation timestamp of the entity
func (m *Machine) SetCreated(created time.Time) {
	m.Created = created
}

// A Machine is a piece of metal which is under the control of our system. It registers itself
// and can be allocated or freed. If the machine is allocated, the substructure Allocation will
// be filled. Any unallocated (free) machine won't have such values.
type Machine struct {
	Base
	Allocation   *MachineAllocation      `rethinkdb:"allocation"`
	PartitionID  string                  `rethinkdb:"partitionid"`
	SizeID       string                  `rethinkdb:"sizeid"`
	RackID       string                  `rethinkdb:"rackid"`
	Waiting      bool                    `rethinkdb:"waiting"`
	PreAllocated bool                    `rethinkdb:"preallocated"`
	Hardware     MachineHardware         `rethinkdb:"hardware"`
	State        MachineState            `rethinkdb:"state"`
	LEDState     ChassisIdentifyLEDState `rethinkdb:"ledstate"`
	Tags         []string                `rethinkdb:"tags"`
	IPMI         IPMI                    `rethinkdb:"ipmi"`
	BIOS         BIOS                    `rethinkdb:"bios"`
}

// A MachineAllocation stores the data which are only present for allocated machines.
type MachineAllocation struct {
	Creator     string    `rethinkdb:"creator"`
	Created     time.Time `rethinkdb:"created"`
	Name        string    `rethinkdb:"name"`
	Description string    `rethinkdb:"description"`
	Project     string    `rethinkdb:"project"`
	ImageID     string    `rethinkdb:"imageid"`
	// FIXME once we implement machine create, store the reference to the fsl instead of the whole copy here
	FilesystemLayoutID string `rethinkdb:"filesystemlayoutid"`
	// FIXME remove and replace with a reference
	FilesystemLayout *FilesystemLayout `rethinkdb:"filesystemlayout"`
	MachineNetworks  []*MachineNetwork `rethinkdb:"networks"`
	Hostname         string            `rethinkdb:"hostname"`
	SSHPubKeys       []string          `rethinkdb:"sshPubKeys"`
	UserData         string            `rethinkdb:"userdata"`
	ConsolePassword  string            `rethinkdb:"console_password"`
	Succeeded        bool              `rethinkdb:"succeeded"`
	Role             Role              `rethinkdb:"role"`
	VPN              *MachineVPN       `rethinkdb:"vpn"`
	UUID             string            `rethinkdb:"uuid"`
	FirewallRules    *FirewallRules    `rethinkdb:"firewall_rules"`
	DNSServers       DNSServers        `rethinkdb:"dns_servers"`
	NTPServers       NTPServers        `rethinkdb:"ntp_servers"`
	Labels           map[string]string `rethinkdb:"labels"`
}

// A MachineState describes the state of a machine. If the Value is AvailableState,
// the machine will be available for allocation. In all other cases the allocation
// must explicitly point to this machine.
type MachineState struct {
	Value              MState `rethinkdb:"value"`
	Description        string `rethinkdb:"description"`
	Issuer             string `rethinkdb:"issuer"`
	MetalHammerVersion string `rethinkdb:"metal_hammer_version"`
}

// A MState is an enum which indicates the state of a machine
type MState string

const (
	// AvailableState describes a machine state where a machine is available for an allocation
	AvailableState MState = ""
	// ReservedState describes a machine state where a machine is not being considered for random allocation
	ReservedState MState = "RESERVED"
	// LockedState describes a machine state where a machine cannot be deleted or allocated anymore
	LockedState MState = "LOCKED"
)

// Role describes the role of a machine.
type Role string

var (
	// RoleMachine is a role that indicates the allocated machine acts as a machine
	RoleMachine Role = "machine"
	// RoleFirewall is a role that indicates the allocated machine acts as a firewall
	RoleFirewall Role = "firewall"
)

type FirewallRules struct {
	Egress  []EgressRule  `rethinkdb:"egress"`
	Ingress []IngressRule `rethinkdb:"ingress"`
}

type EgressRule struct {
	Protocol Protocol `rethinkdb:"protocol"`
	Ports    []int    `rethinkdb:"ports"`
	To       []string `rethinkdb:"to"`
	Comment  string   `rethinkdb:"comment"`
}

type IngressRule struct {
	Protocol Protocol `rethinkdb:"protocol"`
	Ports    []int    `rethinkdb:"ports"`
	To       []string `rethinkdb:"to"`
	From     []string `rethinkdb:"from"`
	Comment  string   `rethinkdb:"comment"`
}

type Protocol string

const (
	ProtocolTCP Protocol = "TCP"
	ProtocolUDP Protocol = "UDP"
)

// func ProtocolFromString(s string) (Protocol, error) {
// 	switch strings.ToLower(s) {
// 	case "tcp":
// 		return ProtocolTCP, nil
// 	case "udp":
// 		return ProtocolUDP, nil
// 	default:
// 		return Protocol(""), fmt.Errorf("no such protocol: %s", s)
// 	}
// }

// func (r EgressRule) Validate() error {
// 	switch r.Protocol {
// 	case ProtocolTCP, ProtocolUDP:
// 		// ok
// 	default:
// 		return fmt.Errorf("egress rule has invalid protocol: %s", r.Protocol)
// 	}

// 	if err := validateComment(r.Comment); err != nil {
// 		return fmt.Errorf("egress rule with error:%w", err)
// 	}
// 	if err := validatePorts(r.Ports); err != nil {
// 		return fmt.Errorf("egress rule with error:%w", err)
// 	}

// 	if err := validateCIDRs(r.To); err != nil {
// 		return fmt.Errorf("egress rule with error:%w", err)
// 	}

// 	return nil
// }

// func (r IngressRule) Validate() error {
// 	switch r.Protocol {
// 	case ProtocolTCP, ProtocolUDP:
// 		// ok
// 	default:
// 		return fmt.Errorf("ingress rule has invalid protocol: %s", r.Protocol)
// 	}
// 	if err := validateComment(r.Comment); err != nil {
// 		return fmt.Errorf("ingress rule with error:%w", err)
// 	}

// 	if err := validatePorts(r.Ports); err != nil {
// 		return fmt.Errorf("ingress rule with error:%w", err)
// 	}
// 	if err := validateCIDRs(r.To); err != nil {
// 		return fmt.Errorf("ingress rule with error:%w", err)
// 	}
// 	if err := validateCIDRs(r.From); err != nil {
// 		return fmt.Errorf("ingress rule with error:%w", err)
// 	}
// 	if err := validateCIDRs(slices.Concat(r.From, r.To)); err != nil {
// 		return fmt.Errorf("ingress rule with error:%w", err)
// 	}

// 	return nil
// }

// const (
// 	allowedCharacters = "abcdefghijklmnopqrstuvwxyz_- "
// 	maxCommentLength  = 100
// )

// func validateComment(comment string) error {
// 	for _, c := range comment {
// 		if !strings.Contains(allowedCharacters, strings.ToLower(string(c))) {
// 			return fmt.Errorf("illegal character in comment found, only: %q allowed", allowedCharacters)
// 		}
// 	}
// 	if len(comment) > maxCommentLength {
// 		return fmt.Errorf("comments can not exceed %d characters", maxCommentLength)
// 	}
// 	return nil
// }

// func validatePorts(ports []int) error {
// 	for _, port := range ports {
// 		if port < 0 || port > 65535 {
// 			return fmt.Errorf("port is out of range")
// 		}
// 	}

// 	return nil
// }

// func validateCIDRs(cidrs []string) error {
// 	var af AddressFamily
// 	for _, cidr := range cidrs {
// 		p, err := netip.ParsePrefix(cidr)
// 		if err != nil {
// 			return fmt.Errorf("invalid cidr: %w", err)
// 		}
// 		var newaf AddressFamily
// 		if p.Addr().Is4() {
// 			newaf = AddressFamilyIPv4
// 		} else if p.Addr().Is6() {
// 			newaf = AddressFamilyIPv6
// 		}
// 		if af != "" && af != newaf {
// 			return fmt.Errorf("mixed address family in one rule is not supported:%v", cidrs)
// 		}
// 		af = newaf
// 	}
// 	return nil
// }

// MachineNetwork stores the Network details of the machine
type MachineNetwork struct {
	NetworkID           string   `rethinkdb:"networkid"`
	Prefixes            []string `rethinkdb:"prefixes"`
	IPs                 []string `rethinkdb:"ips"`
	DestinationPrefixes []string `rethinkdb:"destinationprefixes"`
	Vrf                 uint     `rethinkdb:"vrf"`
	PrivatePrimary      bool     `rethinkdb:"privateprimary"`
	Private             bool     `rethinkdb:"private"`
	ASN                 uint32   `rethinkdb:"asn"`
	Nat                 bool     `rethinkdb:"nat"`
	Underlay            bool     `rethinkdb:"underlay"`
	Shared              bool     `rethinkdb:"shared"`
}

// MachineHardware stores the data which is collected by our system on the hardware when it registers itself.
type MachineHardware struct {
	Memory    uint64        `rethinkdb:"memory"`
	Nics      Nics          `rethinkdb:"network_interfaces"`
	Disks     []BlockDevice `rethinkdb:"block_devices"`
	MetalCPUs []MetalCPU    `rethinkdb:"cpus"`
	MetalGPUs []MetalGPU    `rethinkdb:"gpus"`
}

type MetalCPU struct {
	Vendor  string `rethinkdb:"vendor"`
	Model   string `rethinkdb:"model"`
	Cores   uint32 `rethinkdb:"cores"`
	Threads uint32 `rethinkdb:"threads"`
}

type MetalGPU struct {
	Vendor string `rethinkdb:"vendor"`
	Model  string `rethinkdb:"model"`
}

// MachineLiveliness indicates the liveliness of a machine
type MachineLiveliness string

// The enums for the machine liveliness states.
const (
	MachineLivelinessAlive   MachineLiveliness = "Alive"
	MachineLivelinessDead    MachineLiveliness = "Dead"
	MachineLivelinessUnknown MachineLiveliness = "Unknown"
	MachineDeadAfter         time.Duration     = 5 * time.Minute
	MachineResurrectAfter    time.Duration     = time.Hour
)

// func capacityOf[V any](identifier string, vs []V, countFn func(v V) (model string, count uint64)) (uint64, []V) {
// 	var (
// 		sum     uint64
// 		matched []V
// 	)

// 	for _, v := range vs {
// 		model, count := countFn(v)

// 		if identifier != "" {
// 			matches, err := filepath.Match(identifier, model)
// 			if err != nil {
// 				// illegal identifiers are already prevented by size validation
// 				continue
// 			}

// 			if !matches {
// 				continue
// 			}
// 		}

// 		sum += count
// 		matched = append(matched, v)
// 	}

// 	return sum, matched
// }

// func exhaustiveMatch[V comparable](cs []Constraint, vs []V, countFn func(v V) (model string, count uint64)) bool {
// 	unmatched := slices.Clone(vs)

// 	for _, c := range cs {
// 		capacity, matched := capacityOf(c.Identifier, vs, countFn)

// 		match := c.inRange(capacity)
// 		if !match {
// 			continue
// 		}

// 		unmatched, _ = lo.Difference(unmatched, matched)
// 	}

// 	return len(unmatched) == 0
// }

// // ReadableSpec returns a human readable string for the hardware.
// func (hw *MachineHardware) ReadableSpec() string {
// 	diskCapacity, _ := capacityOf("*", hw.Disks, countDisk)
// 	cpus, _ := capacityOf("*", hw.MetalCPUs, countCPU)
// 	gpus, _ := capacityOf("*", hw.MetalGPUs, countGPU)
// 	return fmt.Sprintf("CPUs: %d, Memory: %s, Storage: %s, GPUs: %d", cpus, humanize.Bytes(hw.Memory), humanize.Bytes(diskCapacity), gpus)
// }

// BlockDevice information.
type BlockDevice struct {
	Name string `rethinkdb:"name"`
	Size uint64 `rethinkdb:"size"`
}

// Fru (Field Replaceable Unit) data
type Fru struct {
	ChassisPartNumber   string `rethinkdb:"chassis_part_number"`
	ChassisPartSerial   string `rethinkdb:"chassis_part_serial"`
	BoardMfg            string `rethinkdb:"board_mfg"`
	BoardMfgSerial      string `rethinkdb:"board_mfg_serial"`
	BoardPartNumber     string `rethinkdb:"board_part_number"`
	ProductManufacturer string `rethinkdb:"product_manufacturer"`
	ProductPartNumber   string `rethinkdb:"product_part_number"`
	ProductSerial       string `rethinkdb:"product_serial"`
}

// IPMI connection data
type IPMI struct {
	// Address is host:port of the connection to the ipmi BMC, host can be either a ip address or a hostname
	Address       string        `rethinkdb:"address"`
	MacAddress    string        `rethinkdb:"mac"`
	User          string        `rethinkdb:"user"`
	Password      string        `rethinkdb:"password"`
	Interface     string        `rethinkdb:"interface"`
	Fru           Fru           `rethinkdb:"fru"`
	BMCVersion    string        `rethinkdb:"bmcversion"`
	PowerState    string        `rethinkdb:"powerstate"`
	PowerMetric   *PowerMetric  `rethinkdb:"powermetric"`
	PowerSupplies PowerSupplies `rethinkdb:"powersupplies"`
	LastUpdated   time.Time     `rethinkdb:"last_updated"`
}

type PowerMetric struct {
	// AverageConsumedWatts shall represent the
	// average power level that occurred averaged over the last IntervalInMin
	// minutes.
	AverageConsumedWatts float32 `rethinkdb:"averageconsumedwatts"`
	// IntervalInMin shall represent the time
	// interval (or window), in minutes, in which the PowerMetrics properties
	// are measured over.
	// Should be an integer, but some Dell implementations return as a float.
	IntervalInMin float32 `rethinkdb:"intervalinmin"`
	// MaxConsumedWatts shall represent the
	// maximum power level in watts that occurred within the last
	// IntervalInMin minutes.
	MaxConsumedWatts float32 `rethinkdb:"maxconsumedwatts"`
	// MinConsumedWatts shall represent the
	// minimum power level in watts that occurred within the last
	// IntervalInMin minutes.
	MinConsumedWatts float32 `rethinkdb:"minconsumedwatts"`
}

type PowerSupplies []PowerSupply
type PowerSupply struct {
	// Status shall contain any status or health properties
	// of the resource.
	Status PowerSupplyStatus `rethinkdb:"status"`
}
type PowerSupplyStatus struct {
	Health string `rethinkdb:"health"`
	State  string `rethinkdb:"state"`
}

// BIOS contains machine bios information
type BIOS struct {
	Version string `rethinkdb:"version"`
	Vendor  string `rethinkdb:"vendor"`
	Date    string `rethinkdb:"date"`
}

type MachineVPN struct {
	ControlPlaneAddress string `rethinkdb:"address"`
	AuthKey             string `rethinkdb:"auth_key"`
	Connected           bool   `rethinkdb:"connected"`
}

// LEDState is the state of the LED of the Machine
type LEDState string

const (
	// LEDStateOn LED is on
	LEDStateOn LEDState = "LED-ON"
	// LEDStateOff LED is off
	LEDStateOff LEDState = "LED-OFF"
)

// LEDStateFrom converts an LEDState string to the corresponding type
func LEDStateFrom(name string) (LEDState, error) {
	switch name {
	case string(LEDStateOff):
		return LEDStateOff, nil
	case string(LEDStateOn):
		return LEDStateOn, nil
	default:
		return "", fmt.Errorf("unknown LEDState:%s", name)
	}
}

// A ChassisIdentifyLEDState describes the state of a chassis identify LED, i.e. LED-ON/LED-OFF.
type ChassisIdentifyLEDState struct {
	Value       LEDState `rethinkdb:"value"`
	Description string   `rethinkdb:"description"`
}

func (n *MachineNetwork) ContainsIP(ip string) bool {
	pip := net.ParseIP(ip)
	for _, p := range n.Prefixes {
		_, n, err := net.ParseCIDR(p)
		if err != nil {
			continue
		}
		if n.Contains(pip) {
			return true
		}
	}
	return false
}
