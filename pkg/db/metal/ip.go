package metal

import (
	"time"
)

// IPType is the type of an ip.
type IPType string

// IPScope is the scope of an ip.
type IPScope string

const (
	// Ephemeral IPs will be cleaned up automatically on machine, network, project deletion
	Ephemeral IPType = "ephemeral"
	// Static IPs will not be cleaned up and can be re-used for machines, networks within a project
	Static IPType = "static"
)

// IP of a machine/firewall.
type IP struct {
	IPAddress string `rethinkdb:"id"`
	// AllocationID will be randomly generated during IP creation and helps identifying the point in time
	// when an IP was created. This is not the primary key!
	// This field can help to distinguish whether an IP address was re-acquired or
	// if it is still the same ip address as before.
	AllocationUUID   string    `rethinkdb:"allocationuuid"`
	ParentPrefixCidr string    `rethinkdb:"prefix"`
	Name             string    `rethinkdb:"name"`
	Description      string    `rethinkdb:"description"`
	ProjectID        string    `rethinkdb:"projectid"`
	NetworkID        string    `rethinkdb:"networkid"`
	Type             IPType    `rethinkdb:"type"`
	Tags             []string  `rethinkdb:"tags"`
	Created          time.Time `rethinkdb:"created"`
	Changed          time.Time `rethinkdb:"changed"`
}

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

// SetChanged sets the last changed timestamp of the entity
func (ip *IP) SetChanged(changed time.Time) {
	ip.Changed = changed
}

// GetCreated returns the creation timestamp of the entity
func (ip *IP) GetCreated() time.Time {
	return ip.Created
}

// SetCreated sets the creation timestamp of the entity
func (ip *IP) SetCreated(created time.Time) {
	ip.Created = created
}
