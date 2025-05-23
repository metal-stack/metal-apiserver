package metal

import (
	"fmt"
	"net/netip"
	"slices"
	"strconv"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/pkg/pointer"
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
		Namespace                  *string           `rethinkdb:"namespace" description:"if this is a namespaced private network, the namespace is stored here, otherwise nil"`
		ParentNetworkID            string            `rethinkdb:"parentnetworkid"`
		Vrf                        uint              `rethinkdb:"vrf"`
		Labels                     map[string]string `rethinkdb:"labels"`
		AdditionalAnnouncableCIDRs []string          `rethinkdb:"additionalannouncablecidrs" description:"list of cidrs which are added to the route maps per tenant private network, these are typically pod- and service cidrs, can only be set in a supernetwork"`
		NetworkType                *NetworkType      `rethinkdb:"networktype"`
		NATType                    *NATType          `rethinkdb:"nattype"`
		// PrivateSuper if set identifies this Network as a Super Network for private networks
		//
		// Deprecated: use SuperNetworkType instead
		PrivateSuper bool `rethinkdb:"privatesuper"`
		// Underlay if set indicates as a underlay network for firewalls and switches
		//
		// Deprecated: use UnderlayNetworkType instead
		Underlay bool `rethinkdb:"underlay"`
		// Shared if set indicates that this network can be used from other projects to acquire ips from
		//
		// Deprecated: use ChildSharedNetworkType instead
		Shared bool `rethinkdb:"shared"`
		// Nat if set, traffic entering this network is masqueraded behind the interface entering this network
		//
		// Deprecated: use IPv4MasqueradeNATType instead
		Nat bool `rethinkdb:"nat"`
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
	// AddressFamilyIPv4 identifies IPv4
	AddressFamilyIPv4 = AddressFamily("IPv4")
	// AddressFamilyIPv6 identifies IPv6
	AddressFamilyIPv6 = AddressFamily("IPv6")

	// NetworkType
	// NetworkTypeExternal identifies a network where ips can be allocated from different projects
	NetworkTypeExternal = NetworkType("external")
	// NetworkTypeUnderlay identifies a underlay network
	NetworkTypeUnderlay = NetworkType("underlay")

	// NetworkTypeSuper identifies a super network where child networks can be allocated from
	NetworkTypeSuper = NetworkType("super")
	// NetworkTypeSuperNamespaced identifies a super network where child networks can be allocated from, namespaced per project
	NetworkTypeSuperNamespaced = NetworkType("super-namespaced")
	// NetworkTypeChild identifies a child network which is only used in one project for machines and firewalls without external connectivity
	NetworkTypeChild = NetworkType("child")
	// NetworkTypeChildShared identifies a child network which can be shared, e.g. ips allocated from different projects
	NetworkTypeChildShared = NetworkType("child-shared")

	// NATType
	NATTypeInvalid = NATType("invalid")
	// NATTypeNone no nat in place when traffic leaves this network
	NATTypeNone = NATType("none")
	// NATTypeIPv4Masquerade masquerade ipv4 behind gateway ip
	NATTypeIPv4Masquerade = NATType("ipv4-masq")
)

func IsSuperNetwork(nt *NetworkType) bool {
	if nt == nil {
		return false
	}
	if *nt == NetworkTypeSuper || *nt == NetworkTypeSuperNamespaced {
		return true
	}
	return false
}

func IsChildNetwork(nt *NetworkType) bool {
	if nt == nil {
		return false
	}
	if *nt == NetworkTypeChild || *nt == NetworkTypeChildShared {
		return true
	}
	return false
}

func ToNetworkType(nwt apiv2.NetworkType) (NetworkType, error) {
	switch nwt {
	case apiv2.NetworkType_NETWORK_TYPE_CHILD:
		return NetworkTypeChild, nil
	case apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED:
		return NetworkTypeChildShared, nil
	case apiv2.NetworkType_NETWORK_TYPE_SUPER:
		return NetworkTypeSuper, nil
	case apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED:
		return NetworkTypeSuperNamespaced, nil
	case apiv2.NetworkType_NETWORK_TYPE_EXTERNAL:
		return NetworkTypeExternal, nil
	case apiv2.NetworkType_NETWORK_TYPE_UNDERLAY:
		return NetworkTypeUnderlay, nil
	case apiv2.NetworkType_NETWORK_TYPE_UNSPECIFIED:
		return NetworkType(""), fmt.Errorf("given networkType:%q is invalid", nwt)
	}
	return NetworkType(""), fmt.Errorf("given networkType:%q is invalid", nwt)
}

func FromNetworkType(nwt NetworkType) (apiv2.NetworkType, error) {
	apiv2NetworkType, err := enum.GetEnum[apiv2.NetworkType](string(nwt))
	if err != nil {
		return apiv2.NetworkType_NETWORK_TYPE_UNSPECIFIED, fmt.Errorf("given networkType:%q is invalid", nwt)
	}
	return apiv2NetworkType, nil
}

func ToNATType(nt apiv2.NATType) (NATType, error) {
	switch nt {
	case apiv2.NATType_NAT_TYPE_NONE:
		return NATTypeNone, nil
	case apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE:
		return NATTypeIPv4Masquerade, nil
	case apiv2.NATType_NAT_TYPE_UNSPECIFIED:
		return NATTypeInvalid, fmt.Errorf("given natType:%q is invalid", nt)
	}
	return NATTypeInvalid, fmt.Errorf("given natType:%q is invalid", nt)
}

func FromNATType(nt NATType) (apiv2.NATType, error) {
	apiv2NatType, err := enum.GetEnum[apiv2.NATType](string(nt))
	if err != nil {
		return apiv2.NATType_NAT_TYPE_UNSPECIFIED, fmt.Errorf("given nat type %q is invalid", nt)
	}
	return apiv2NatType, nil
}

func ToAddressFamily(af apiv2.IPAddressFamily) (AddressFamily, error) {
	switch af {
	case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4:
		return AddressFamilyIPv4, nil
	case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6:
		return AddressFamilyIPv6, nil
	case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_UNSPECIFIED:
		return "", fmt.Errorf("given addressfamily %q is invalid", af)
	}

	return "", fmt.Errorf("given addressfamily %q is invalid", af)
}

// ToAddressFamilyFromNetwork returns the metal address family of the corresponding apiv2 address family.
// Attention: this function might return nil for network family dual stack!!
func ToAddressFamilyFromNetwork(af apiv2.NetworkAddressFamily) (*AddressFamily, error) {
	switch af {
	case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_DUAL_STACK:
		return nil, nil
	case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V4:
		return pointer.Pointer(AddressFamilyIPv4), nil
	case apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_V6:
		return pointer.Pointer(AddressFamilyIPv6), nil
	default:
		return nil, fmt.Errorf("given addressfamily %q is invalid", af)
	}
}

func ToChildPrefixLength(cpl *apiv2.ChildPrefixLength, prefixes Prefixes) (ChildPrefixLength, error) {
	childPrefixLength := ChildPrefixLength{}
	if cpl == nil {
		return nil, nil
	}

	if cpl.Ipv4 != nil {
		if !slices.Contains(prefixes.AddressFamilies(), AddressFamilyIPv4) {
			return nil, fmt.Errorf("childprefixlength for addressfamily: %q specified, but no %q addressfamily found in prefixes", AddressFamilyIPv4, AddressFamilyIPv4)
		}
		childPrefixLength[AddressFamilyIPv4] = uint8(*cpl.Ipv4)
	}

	if cpl.Ipv6 != nil {
		if !slices.Contains(prefixes.AddressFamilies(), AddressFamilyIPv6) {
			return nil, fmt.Errorf("childprefixlength for addressfamily: %q specified, but no %q addressfamily found in prefixes", AddressFamilyIPv6, AddressFamilyIPv6)
		}
		childPrefixLength[AddressFamilyIPv6] = uint8(*cpl.Ipv6)
	}

	for af, length := range childPrefixLength {
		// check if childprefixlength is set and matches addressfamily
		for _, p := range prefixes.OfFamily(af) {
			ipprefix, err := netip.ParsePrefix(p.String())
			if err != nil {
				return nil, fmt.Errorf("given prefix %q is not a valid ip with mask: %w", p.String(), err)
			}
			if int(length) <= ipprefix.Bits() {
				return nil, fmt.Errorf("given childprefixlength %d is not greater than prefix length of:%s", length, p.String())
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

		if pfx.Addr().Is4() && af == AddressFamilyIPv6 {
			continue
		}
		if pfx.Addr().Is6() && af == AddressFamilyIPv4 {
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
			af = AddressFamilyIPv4
		}
		if pfx.Addr().Is6() {
			af = AddressFamilyIPv6
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
	return slices.DeleteFunc(n.Prefixes, func(a Prefix) bool {
		return slices.ContainsFunc(prefixes, func(b Prefix) bool {
			return a.equals(&b)
		})
	})
}
