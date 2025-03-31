package metal

import (
	"fmt"
	"net/netip"
	"slices"
	"strconv"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
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

func ToAddressFamily(af apiv2.IPAddressFamily) (AddressFamily, error) {
	switch af {
	case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4:
		return IPv4AddressFamily, nil
	case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6:
		return IPv6AddressFamily, nil
	case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_UNSPECIFIED:
		return InvalidAddressFamily, fmt.Errorf("given addressfamily:%q is invalid", af)
	}
	return InvalidAddressFamily, fmt.Errorf("given addressfamily:%q is invalid", af)
}

func ToDefaultChildPrefixLength(pfs []*apiv2.ChildPrefixLength, prefixes Prefixes) (ChildPrefixLength, error) {
	result := ChildPrefixLength{}
	for _, pf := range pfs {
		af, err := ToAddressFamily(pf.AddressFamily)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(prefixes.AddressFamilies(), af) {
			return nil, fmt.Errorf("no addressfamily %q present for defaultchildprefixlength: %d", af, pf.Length)
		}

		result[af] = uint8(pf.Length)
	}
	return result, nil
}

func (p *Prefix) String() string {
	return p.IP + "/" + p.Length
}

// equals returns true when prefixes have the same cidr.
func (p *Prefix) equals(other *Prefix) bool {
	return p.String() == other.String()
}

func (p Prefixes) String() []string {
	result := []string{}
	for _, element := range p {
		result = append(result, element.String())
	}
	return result
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

func NewPrefixesFromCIDRs(cidrs []string) (Prefixes, error) {
	var (
		result Prefixes
	)
	for _, p := range cidrs {
		prefix, _, err := NewPrefixFromCIDR(p)
		if err != nil {
			return nil, err
		}
		result = append(result, *prefix)
	}
	return result, nil
}

// NewPrefixFromCIDR returns a new prefix from a given cidr.
func NewPrefixFromCIDR(cidr string) (*Prefix, *netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, nil, fmt.Errorf("given cidr %q is not a valid ip with mask: %w", cidr, err)
	}
	ip := prefix.Addr().String()
	length := strconv.Itoa(prefix.Bits())
	return &Prefix{
		IP:     ip,
		Length: length,
	}, &prefix, nil
}

// SubtractPrefixes returns the prefixes of the network minus the prefixes passed in the arguments
func (n *Network) SubtractPrefixes(prefixes ...Prefix) []Prefix {
	var result []Prefix
	for _, p := range n.Prefixes {
		contains := false
		for i := range prefixes {
			if p.equals(&prefixes[i]) {
				contains = true
				break
			}
		}
		if contains {
			continue
		}
		result = append(result, p)
	}
	return result
}
