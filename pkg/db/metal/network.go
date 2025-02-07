package metal

type (
	Network struct {
		Base
		Prefixes                   Prefixes          `rethinkdb:"prefixes"`
		DestinationPrefixes        Prefixes          `rethinkdb:"destinationprefixes"`
		DefaultChildPrefixLength   ChildPrefixLength `rethinkdb:"defaultchildprefixlength" description:"if privatesuper, this defines the bitlen of child prefixes per addressfamily if not nil"`
		PartitionID                string            `rethinkdb:"partitionid"`
		ProjectID                  string            `rethinkdb:"projectid"`
		ParentNetworkID            string            `rethinkdb:"parentnetworkid"`
		Vrf                        uint              `rethinkdb:"vrf"`
		PrivateSuper               bool              `rethinkdb:"privatesuper"`
		Nat                        bool              `rethinkdb:"nat"`
		Underlay                   bool              `rethinkdb:"underlay"`
		Shared                     bool              `rethinkdb:"shared"`
		Labels                     map[string]string `rethinkdb:"labels"`
		AddressFamilies            AddressFamilies   `rethinkdb:"addressfamilies"`
		AdditionalAnnouncableCIDRs []string          `rethinkdb:"additionalannouncablecidrs" description:"list of cidrs which are added to the route maps per tenant private network, these are typically pod- and service cidrs, can only be set in a supernetwork"`
	}

	ChildPrefixLength map[AddressFamily]uint8

	// AddressFamily identifies IPv4/IPv6
	AddressFamily   string
	AddressFamilies []AddressFamily

	Prefix struct {
		IP     string `rethinkdb:"ip"`
		Length string `rethinkdb:"length"`
	}

	// Prefixes is an array of prefixes
	Prefixes []Prefix
)
