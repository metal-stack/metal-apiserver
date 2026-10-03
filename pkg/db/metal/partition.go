package metal

// A Partition represents a location.
type Partition struct {
	Base
	BootConfiguration  BootConfiguration `rethinkdb:"bootconfig" json:"boot_configuration"`
	MgmtServiceAddress string            `rethinkdb:"mgmtserviceaddr" json:"mgmt_service_address"`
	Labels             map[string]string `rethinkdb:"labels" json:"labels"`
	DNSServers         DNSServers        `rethinkdb:"dns_servers" json:"dns_servers"`
	NTPServers         NTPServers        `rethinkdb:"ntp_servers" json:"ntp_servers"`
}

// BootConfiguration defines the metal-hammer initrd, kernel and commandline
type BootConfiguration struct {
	ImageURL    string `rethinkdb:"imageurl" json:"image_url"`
	KernelURL   string `rethinkdb:"kernelurl" json:"kernel_url"`
	CommandLine string `rethinkdb:"commandline" json:"command_line"`
}

// PartitionsByID creates an indexed map of partitions where the id is the index.
func PartitionsByID(partitions []*Partition) map[string]*Partition {
	res := make(map[string]*Partition)
	for i, s := range partitions {
		res[s.ID] = partitions[i]
	}
	return res
}
