package repository

import (
	"context"
	"net/netip"
	"slices"
	"strings"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

// ipamRootNamespace is the namespace go-ipam uses when no namespace is provided.
// Networks and IPs without a namespace are stored in this namespace in ipam.
const ipamRootNamespace = "root"

type (
	networkConsistencyKey struct {
		// namespace is normalized, the root namespace is represented by an empty string.
		namespace string
		cidr      string
	}

	ipamPrefix struct {
		cidr      string
		namespace *string
	}

	ipConsistencyKey struct {
		namespace string
		cidr      string
		ip        string
	}

	ipamIP struct {
		ip        string
		cidr      string
		namespace *string
	}
)

// NetworksConsistent compares the networks and ips stored in the metal database with
// the prefixes and ips stored in ipam. It returns all entities which only exist in one
// of both systems.
func (r *networkRepository) NetworksConsistent(ctx context.Context) (*adminv2.NetworkServiceIssuesResponse, error) {
	r.s.log.Info("running network consistency check")
	metalNetworks, err := r.s.ds.Network().List(ctx, queries.NetworkFilter(&apiv2.NetworkQuery{}))
	if err != nil {
		return nil, err
	}

	ipamPrefixes, err := r.listIPAMPrefixes(ctx)
	if err != nil {
		return nil, err
	}

	var (
		metalNetworksByPrefix = make(map[networkConsistencyKey]*metal.Network)
		// Parent prefixes of namespaced super networks are copied into the namespace of
		// each child network in ipam. These copies do not have a corresponding metal network.
		namespacedSuperPrefixes = make(map[string]struct{})
	)

	for _, mn := range metalNetworks {
		ns := normalizeNetworkNamespace(mn.Namespace)
		for _, prefix := range mn.Prefixes {
			key := networkConsistencyKey{namespace: ns, cidr: prefix.String()}
			if _, exists := metalNetworksByPrefix[key]; !exists {
				metalNetworksByPrefix[key] = mn
			}
		}
		if mn.NetworkType == metal.NetworkTypeSuperNamespaced {
			for _, prefix := range mn.Prefixes {
				namespacedSuperPrefixes[prefix.String()] = struct{}{}
			}
		}
	}

	response := &adminv2.NetworkServiceIssuesResponse{}

	for _, mn := range metalNetworks {
		ns := normalizeNetworkNamespace(mn.Namespace)
		missingInIPAM := false
		for _, prefix := range mn.Prefixes {
			key := networkConsistencyKey{namespace: ns, cidr: prefix.String()}
			if _, exists := ipamPrefixes[key]; !exists {
				missingInIPAM = true
				break
			}
		}
		if !missingInIPAM {
			continue
		}

		network, err := r.convertToProto(ctx, mn)
		if err != nil {
			return nil, err
		}
		response.NetworksMissingInIpam = append(response.NetworksMissingInIpam, network)
	}

	for key, prefix := range ipamPrefixes {
		if _, exists := metalNetworksByPrefix[key]; exists {
			continue
		}
		if key.namespace != "" {
			if _, exists := namespacedSuperPrefixes[prefix.cidr]; exists {
				continue
			}
		}

		response.NetworksMissingInMetal = append(response.NetworksMissingInMetal, &apiv2.Network{
			Id:        prefix.cidr,
			Prefixes:  []string{prefix.cidr},
			Namespace: normalizedNamespacePointer(prefix.namespace),
			Meta:      &apiv2.Meta{},
		})
	}

	metalIPs, err := r.s.ds.IP().List(ctx, queries.IpFilter(&apiv2.IPQuery{}))
	if err != nil {
		return nil, err
	}

	ipamIPs, err := r.listIPAMIPs(ctx, ipamPrefixes)
	if err != nil {
		return nil, err
	}

	metalIPsByKey := make(map[ipConsistencyKey]*metal.IP, len(metalIPs))
	for _, ip := range metalIPs {
		address, err := ip.GetIPAddress()
		if err != nil {
			return nil, err
		}
		key := ipConsistencyKey{
			namespace: normalizeNetworkNamespace(ip.Namespace),
			cidr:      ip.ParentPrefixCidr,
			ip:        address,
		}
		metalIPsByKey[key] = ip
	}

	ipRepository := &ipRepository{s: r.s}
	for _, ip := range metalIPs {
		address, err := ip.GetIPAddress()
		if err != nil {
			return nil, err
		}
		key := ipConsistencyKey{
			namespace: normalizeNetworkNamespace(ip.Namespace),
			cidr:      ip.ParentPrefixCidr,
			ip:        address,
		}
		if _, exists := ipamIPs[key]; exists {
			continue
		}

		ipProto, err := ipRepository.convertToProto(ctx, ip)
		if err != nil {
			return nil, err
		}
		response.IpsMissingInIpam = append(response.IpsMissingInIpam, ipProto)
	}

	for key, ip := range ipamIPs {
		if _, exists := metalIPsByKey[key]; exists {
			continue
		}

		networkID := ""
		networkKey := networkConsistencyKey{namespace: key.namespace, cidr: key.cidr}
		if nw, exists := metalNetworksByPrefix[networkKey]; exists {
			networkID = nw.ID
		}

		response.IpsMissingInMetal = append(response.IpsMissingInMetal, &apiv2.IP{
			Ip:        ip.ip,
			Network:   networkID,
			Namespace: normalizedNamespacePointer(ip.namespace),
			Meta:      &apiv2.Meta{},
		})
	}

	sortNetworksConsistentResponse(response)

	return response, nil
}

func (r *networkRepository) listIPAMPrefixes(ctx context.Context) (map[networkConsistencyKey]ipamPrefix, error) {
	result := make(map[networkConsistencyKey]ipamPrefix)

	// Always list the root namespace. In ipam the root namespace is called differently
	// than the empty namespace used in the metal database, so normalize it to the
	// empty namespace which means "not namespaced" from the metal database perspective.
	prefixes, err := r.s.ipam.ListPrefixes(ctx, &ipamv1.ListPrefixesRequest{})
	if err != nil {
		return nil, err
	}
	for _, prefix := range prefixes.Prefixes {
		result[networkConsistencyKey{namespace: "", cidr: prefix.Cidr}] = ipamPrefix{cidr: prefix.Cidr}
	}

	namespaces, err := r.s.ipam.ListNamespaces(ctx, &ipamv1.ListNamespacesRequest{})
	if err != nil {
		return nil, err
	}
	for _, namespace := range namespaces.Namespace {
		if normalizeNetworkNamespace(&namespace) == "" {
			// root namespace is already listed above.
			continue
		}

		namespace := namespace
		prefixes, err := r.s.ipam.ListPrefixes(ctx, &ipamv1.ListPrefixesRequest{
			Namespace: &namespace,
		})
		if err != nil {
			return nil, err
		}
		for _, prefix := range prefixes.Prefixes {
			result[networkConsistencyKey{namespace: namespace, cidr: prefix.Cidr}] = ipamPrefix{
				cidr:      prefix.Cidr,
				namespace: &namespace,
			}
		}
	}

	return result, nil
}

func (r *networkRepository) listIPAMIPs(ctx context.Context, prefixes map[networkConsistencyKey]ipamPrefix) (map[ipConsistencyKey]ipamIP, error) {
	result := make(map[ipConsistencyKey]ipamIP)

	for key, prefix := range prefixes {
		ips, err := r.s.ipam.ListIPs(ctx, &ipamv1.ListIPsRequest{
			PrefixCidr: prefix.cidr,
			Namespace:  prefix.namespace,
		})
		if err != nil {
			return nil, err
		}

		for _, ip := range ips.Ip {
			reserved, err := isReservedIPAMIP(prefix.cidr, ip)
			if err != nil {
				return nil, err
			}
			if reserved {
				// go-ipam always reserves the first ip of a prefix and the broadcast
				// address of an IPv4 prefix. These cannot be allocated in metal.
				continue
			}

			result[ipConsistencyKey{
				namespace: key.namespace,
				cidr:      prefix.cidr,
				ip:        ip,
			}] = ipamIP{
				ip:        ip,
				cidr:      prefix.cidr,
				namespace: prefix.namespace,
			}
		}
	}

	return result, nil
}

func isReservedIPAMIP(cidr, address string) (bool, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return false, err
	}
	ip, err := netip.ParseAddr(address)
	if err != nil {
		return false, err
	}

	prefix = prefix.Masked()
	if ip == prefix.Addr() {
		return true, nil
	}

	if ip.Is4() {
		broadcast := prefix.Addr().As4()
		for i := uint8(0); i < 32-uint8(prefix.Bits()); i++ {
			broadcast[3-i/8] |= 1 << (i % 8)
		}
		if ip == netip.AddrFrom4(broadcast) {
			return true, nil
		}
	}

	return false, nil
}

// normalizeNetworkNamespace maps the ipam root namespace and an empty namespace to the
// same value, because a network without a namespace is stored in the ipam root namespace.
func normalizeNetworkNamespace(namespace *string) string {
	ns := pointer.SafeDeref(namespace)
	if ns == ipamRootNamespace {
		return ""
	}
	return ns
}

// normalizedNamespacePointer returns nil for the root namespace so that it matches the
// representation of non-namespaced networks in the metal database.
func normalizedNamespacePointer(namespace *string) *string {
	ns := normalizeNetworkNamespace(namespace)
	if ns == "" {
		return nil
	}
	return &ns
}

func sortNetworksConsistentResponse(response *adminv2.NetworkServiceIssuesResponse) {
	slices.SortFunc(response.NetworksMissingInIpam, func(a, b *apiv2.Network) int {
		return strings.Compare(a.Id, b.Id)
	})
	slices.SortFunc(response.NetworksMissingInMetal, func(a, b *apiv2.Network) int {
		if c := strings.Compare(pointer.SafeDeref(a.Namespace), pointer.SafeDeref(b.Namespace)); c != 0 {
			return c
		}
		return strings.Compare(a.Id, b.Id)
	})
	slices.SortFunc(response.IpsMissingInIpam, func(a, b *apiv2.IP) int {
		if c := strings.Compare(pointer.SafeDeref(a.Namespace), pointer.SafeDeref(b.Namespace)); c != 0 {
			return c
		}
		if c := strings.Compare(a.Network, b.Network); c != 0 {
			return c
		}
		return strings.Compare(a.Ip, b.Ip)
	})
	slices.SortFunc(response.IpsMissingInMetal, func(a, b *apiv2.IP) int {
		if c := strings.Compare(pointer.SafeDeref(a.Namespace), pointer.SafeDeref(b.Namespace)); c != 0 {
			return c
		}
		if c := strings.Compare(a.Network, b.Network); c != 0 {
			return c
		}
		return strings.Compare(a.Ip, b.Ip)
	})
}
