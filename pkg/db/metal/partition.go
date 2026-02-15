package metal

// A Partition represents a location.
type Partition struct {
	Base
	BootConfiguration  BootConfiguration `rethinkdb:"bootconfig"`
	MgmtServiceAddress string            `rethinkdb:"mgmtserviceaddr"`
	Labels             map[string]string `rethinkdb:"labels"`
	DNSServers         DNSServers        `rethinkdb:"dns_servers"`
	NTPServers         NTPServers        `rethinkdb:"ntp_servers"`
}

// BootConfiguration defines the metal-hammer initrd, kernel and commandline
type BootConfiguration struct {
	ImageURL    string `rethinkdb:"imageurl"`
	KernelURL   string `rethinkdb:"kernelurl"`
	CommandLine string `rethinkdb:"commandline"`
}

// PartitionsByID creates an indexed map of partitions where the id is the index.
func PartitionsByID(partitions []*Partition) map[string]*Partition {
	res := make(map[string]*Partition)
	for i, s := range partitions {
		res[s.ID] = partitions[i]
	}
	return res
}
