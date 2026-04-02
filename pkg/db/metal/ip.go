package metal

import (
	"fmt"
	"strings"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/tags"
	"github.com/metal-stack/metal-lib/pkg/tag"
)

// IPType is the type of an ip.
type IPType string

// IPScope is the scope of an ip.
type IPScope string

const (
	// TagIPSeparator is the separator character for key and values in IP-Tags
	TagIPSeparator = "="
	// Ephemeral IPs will be cleaned up automatically on machine, network, project deletion
	Ephemeral IPType = "ephemeral"
	// Static IPs will not be cleaned up and can be re-used for machines, networks within a project
	Static IPType = "static"

	// ScopeEmpty IPs are not bound to a project, machine or cluster
	ScopeEmpty IPScope = ""
	// ScopeProject IPs can be assigned to machines or used by cluster services
	ScopeProject IPScope = "project"
	// ScopeMachine IPs are bound to the usage directly at machines
	ScopeMachine IPScope = "machine"
)

// IP of a machine/firewall.
type IP struct {
	// IPAddress is stored either as the plain IP or prefixed with the namespace if the namespace is not nil
	IPAddress string `rethinkdb:"id"`
	// AllocationID will be randomly generated during IP creation and helps identifying the point in time
	// when an IP was created. This is not the primary key!
	// This field can help to distinguish whether an IP address was re-acquired or
	// if it is still the same ip address as before.
	AllocationUUID   string    `rethinkdb:"allocationuuid"`
	Namespace        *string   `rethinkdb:"namespace" description:"if this is a ip in a namespaced private network, the namespace is stored here, otherwise nil"`
	ParentPrefixCidr string    `rethinkdb:"prefix"`
	Name             string    `rethinkdb:"name"`
	Description      string    `rethinkdb:"description"`
	ProjectID        string    `rethinkdb:"projectid"`
	NetworkID        string    `rethinkdb:"networkid"`
	Type             IPType    `rethinkdb:"type"`
	Tags             []string  `rethinkdb:"tags"`
	Created          time.Time `rethinkdb:"created"`
	Changed          time.Time `rethinkdb:"changed"`
	Generation       uint64    `rethinkdb:"generation"`
}

type IPs []*IP
type IPsMap map[string]IPs

// GetID returns the ID of the entity
func (ip *IP) GetID() string {
	return ip.IPAddress
}

// SetID sets the ID of the entity
func (ip *IP) SetID(id string) {
	ip.IPAddress = id
}

// GetChanged returns the last changed timestamp of the entity
func (ip *IP) GetChanged() time.Time {
	return ip.Changed
}

// GetCreated returns the creation timestamp of the entity
func (ip *IP) GetCreated() time.Time {
	return ip.Created
}

// GetGeneration returns the generation of the entity
func (ip *IP) GetGeneration() uint64 {
	return ip.Generation
}

func (ip *IP) SetChanged(t time.Time) {
	ip.Changed = t
}

// ---------------

const namespaceSeparator = "-"

// GetIPAddress returns the IPAddress of this IP without namespace if namespaced
func (ip *IP) GetIPAddress() (string, error) {
	if ip.Namespace == nil {
		return ip.IPAddress, nil
	}
	ipaddress, found := strings.CutPrefix(ip.IPAddress, *ip.Namespace+namespaceSeparator)
	if found {
		return ipaddress, nil
	}
	return "", errorutil.Internal("ip %q is namespaced, but namespace not stored in ip field", ip.IPAddress)
}

func CreateNamespacedIPAddress(namespace *string, ip string) string {
	if namespace == nil {
		return ip
	}
	return fmt.Sprintf("%s%s%s", *namespace, namespaceSeparator, ip)
}

func ToIPType(ipt *apiv2.IPType) (IPType, error) {
	if ipt == nil {
		return Ephemeral, nil
	}

	switch *ipt {
	case apiv2.IPType_IP_TYPE_EPHEMERAL:
		return Ephemeral, nil
	case apiv2.IPType_IP_TYPE_STATIC:
		return Static, nil
	case apiv2.IPType_IP_TYPE_UNSPECIFIED:
		fallthrough
	default:
		return Ephemeral, errorutil.InvalidArgument("given ip type is not supported:%s", ipt.String())
	}
}

func IPsByProject(ips []*IP) IPsMap {
	ipMap := make(IPsMap)
	for _, ip := range ips {
		ipMap[ip.ProjectID] = append(ipMap[ip.ProjectID], ip)
	}
	return ipMap
}

// GetScope determines the scope of an ip address
// This is important during machine creation.
// If a machine creation gets a ip passed which was already allocated for another machine,
// it must not be used for another machine network
// Ips will get a tag with the machine id if it is one of the machine network main ips
// Ips will get a tag with the project id if it is acquired for other purposes like service type loadbalancers
func (ip *IP) GetScope() IPScope {
	if ip.ProjectID == "" {
		return ScopeEmpty
	}
	for _, t := range ip.Tags {
		if strings.HasPrefix(t, tag.MachineID) {
			return ScopeMachine
		}
	}
	return ScopeProject
}

func (ip *IP) HasMachineId(id string) bool {
	t := tags.New(ip.Tags)
	return t.Has(IpTag(tag.MachineID, id))
}

func (ip *IP) GetMachineIds() []string {
	ts := tags.New(ip.Tags)
	return ts.Values(tag.MachineID + TagIPSeparator)
}

func (ip *IP) AddMachineId(id string) {
	ts := tags.New(ip.Tags)
	t := IpTag(tag.MachineID, id)
	ts.Remove(tag.MachineID)
	ts.Add(t)
	ip.Tags = ts.Unique()
}

func (ip *IP) RemoveMachineId(id string) {
	ts := tags.New(ip.Tags)
	t := IpTag(tag.MachineID, id)
	ts.Remove(t)
	ip.Tags = ts.Unique()
}

func IpTag(key, value string) string {
	return fmt.Sprintf("%s%s%s", key, TagIPSeparator, value)
}
