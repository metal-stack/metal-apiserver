package metal

import (
	"net/netip"
	"slices"
)

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

const (
	// InvalidAddressFamily identifies a invalid Addressfamily
	InvalidAddressFamily = AddressFamily("invalid")
	// IPv4AddressFamily identifies IPv4
	IPv4AddressFamily = AddressFamily("IPv4")
	// IPv6AddressFamily identifies IPv6
	IPv6AddressFamily = AddressFamily("IPv6")
)

func (p *Prefix) String() string {
	return p.IP + "/" + p.Length
}

// OfFamily returns the prefixes of the given address family.
// be aware that malformed prefixes are just skipped, so do not use this for validation or something.
func (p Prefixes) OfFamily(af AddressFamily) Prefixes {
	var res Prefixes

	for _, prefix := range p {
		pfx, err := netip.ParsePrefix(prefix.String())
		if err != nil {
			continue
		}

		if pfx.Addr().Is4() && af == IPv6AddressFamily {
			continue
		}
		if pfx.Addr().Is6() && af == IPv4AddressFamily {
			continue
		}

		res = append(res, prefix)
	}

	return res
}

// AddressFamilies returns the addressfamilies of given prefixes.
// be aware that malformed prefixes are just skipped, so do not use this for validation or something.
func (p Prefixes) AddressFamilies() AddressFamilies {
	var afs AddressFamilies

	for _, prefix := range p {
		pfx, err := netip.ParsePrefix(prefix.String())
		if err != nil {
			continue
		}

		var af AddressFamily
		if pfx.Addr().Is4() {
			af = IPv4AddressFamily
		}
		if pfx.Addr().Is6() {
			af = IPv6AddressFamily
		}
		if !slices.Contains(afs, af) {
			afs = append(afs, af)
		}
	}

	return afs
}
