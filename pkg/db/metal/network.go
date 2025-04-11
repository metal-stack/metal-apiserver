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
		MinChildPrefixLength       ChildPrefixLength `rethinkdb:"minchildprefixlength" description:"if privatesuper, this defines the minimum bitlen of child prefixes per addressfamily if not nil"`
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
		NetworkType                *NetworkType      `rethinkdb:"networkType"`
		NATType                    *NATType          `rethinkdb:"natType"`
	}

	NATType     string
	NetworkType string

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

	// NetworkType

	// InvalidNetworkType identifies a invalid network
	InvalidNetworkType = NetworkType("invalid")
	// SharedNetworkType identifies a network where ips can be allocated from different projects
	SharedNetworkType = NetworkType("shared")
	// UnderlayNetworkType identifies a underlay network
	UnderlayNetworkType = NetworkType("underlay")
	// SuperVrfSharedNetworkType identifies a private super network where private networks can be allocated from but they will share the vrf ids with its super
	SuperVrfSharedNetworkType = NetworkType("private-super-shared-vrf")
	// VrfSharedNetworkType identifies a private network with shares vrf ids with other private networks
	VrfSharedNetworkType = NetworkType("private-shared-vrf")

	// PrivateSuperNetworkType identifies a private super network where private networks can be allocated from
	PrivateSuperNetworkType = NetworkType("private-super")
	// PrivateNetworkType identifies a private network which is only used in one project for machines and firewalls without external connectivity
	PrivateNetworkType = NetworkType("private")
	// PrivateSharedNetworkType identifies a private network which can be shared, e.g. ips allocated from different projects
	PrivateSharedNetworkType = NetworkType("private-shared")

	// NATType
	InvalidNATType = NATType("invalid")
	// NoneNATType no nat in place when traffic leaves this network
	NoneNATType = NATType("none")
	// IPv4MasqueradeNATType masquerade ipv4 behind gateway ip
	IPv4MasqueradeNATType = NATType("ipv4-masquerade")
)

func ToNetworkTyp(nwt apiv2.NetworkType) (NetworkType, error) {
	switch nwt {
	case apiv2.NetworkType_NETWORK_TYPE_PRIVATE:
		return PrivateNetworkType, nil
	case apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SHARED:
		return PrivateSharedNetworkType, nil
	case apiv2.NetworkType_NETWORK_TYPE_VRF_SHARED:
		return VrfSharedNetworkType, nil
	case apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER:
		return PrivateSuperNetworkType, nil
	case apiv2.NetworkType_NETWORK_TYPE_SUPER_VRF_SHARED:
		return SuperVrfSharedNetworkType, nil
	case apiv2.NetworkType_NETWORK_TYPE_SHARED:
		return SharedNetworkType, nil
	case apiv2.NetworkType_NETWORK_TYPE_UNDERLAY:
		return UnderlayNetworkType, nil
	case apiv2.NetworkType_NETWORK_TYPE_UNSPECIFIED:
		// TODO: Shared network is default if none is specified, should we make this a failure
		return SharedNetworkType, nil
	}
	return InvalidNetworkType, fmt.Errorf("given networkType:%q is invalid", nwt)
}

func ToNATType(nt apiv2.NATType) (NATType, error) {
	switch nt {
	case apiv2.NATType_NAT_TYPE_NONE:
		return NoneNATType, nil
	case apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE:
		return IPv4MasqueradeNATType, nil
	case apiv2.NATType_NAT_TYPE_UNSPECIFIED:
		return InvalidNATType, fmt.Errorf("given natType:%q is invalid", nt)
	}
	return InvalidNATType, fmt.Errorf("given natType:%q is invalid", nt)
}

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

func ToChildPrefixLength(cpl *apiv2.ChildPrefixLength, prefixes Prefixes) (ChildPrefixLength, error) {

	childPrefixLength := ChildPrefixLength{}
	if cpl == nil {
		return nil, nil
	}

	if cpl.Ipv4 != nil {
		if !slices.Contains(prefixes.AddressFamilies(), IPv4AddressFamily) {
			return nil, fmt.Errorf("childprefixlength for addressfamily: %q specified, but no %q addressfamily found in prefixes", IPv4AddressFamily, IPv4AddressFamily)
		}
		childPrefixLength[IPv4AddressFamily] = uint8(*cpl.Ipv4)
	}

	if cpl.Ipv6 != nil {
		if !slices.Contains(prefixes.AddressFamilies(), IPv6AddressFamily) {
			return nil, fmt.Errorf("childprefixlength for addressfamily: %q specified, but no %q addressfamily found in prefixes", IPv6AddressFamily, IPv6AddressFamily)
		}
		childPrefixLength[IPv6AddressFamily] = uint8(*cpl.Ipv6)
	}

	for af, length := range childPrefixLength {
		// check if childprefixlength is set and matches addressfamily
		for _, p := range prefixes.OfFamily(af) {
			ipprefix, err := netip.ParsePrefix(p.String())
			if err != nil {
				return nil, fmt.Errorf("given prefix %q is not a valid ip with mask: %w", p.String(), err)
			}
			if int(length) <= ipprefix.Bits() {
				return nil, fmt.Errorf("given defaultchildprefixlength %d is not greater than prefix length of:%s", length, p.String())
			}
		}
	}

	return childPrefixLength, nil
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
	slices.Sort(result)
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
