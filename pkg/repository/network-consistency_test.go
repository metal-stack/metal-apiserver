package repository_test

import (
	"testing"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestNetworkRepositoryNetworksConsistent(t *testing.T) {
	ctx := t.Context()
	testStore, closer := test.StartRepositoryWithCleanup(t)
	defer closer()

	ds := testStore.GetDatastore()
	ipam := testStore.GetIpamClient()

	tenantNamespace := new("tenant-a")

	metalNetworks := []*metal.Network{
		{
			ID:          "consistent-root",
			Prefixes:    metal.Prefixes{{IP: "10.0.0.0", Length: "24"}},
			NetworkType: metal.NetworkTypeExternal,
			NATType:     metal.NATTypeNone,
		},
		{
			ID:          "missing-in-ipam",
			Prefixes:    metal.Prefixes{{IP: "10.0.1.0", Length: "24"}},
			NetworkType: metal.NetworkTypeExternal,
			NATType:     metal.NATTypeNone,
		},
		{
			ID:          "consistent-tenant",
			Prefixes:    metal.Prefixes{{IP: "10.0.0.0", Length: "24"}},
			Namespace:   tenantNamespace,
			NetworkType: metal.NetworkTypeChild,
			NATType:     metal.NATTypeNone,
		},
		{
			ID:          "namespaced-super",
			Prefixes:    metal.Prefixes{{IP: "10.10.0.0", Length: "16"}},
			NetworkType: metal.NetworkTypeSuperNamespaced,
			NATType:     metal.NATTypeNone,
		},
	}
	for _, nw := range metalNetworks {
		_, err := ds.Network().Create(ctx, nw)
		require.NoError(t, err)
	}

	_, err := ipam.CreateNamespace(ctx, &ipamv1.CreateNamespaceRequest{Namespace: *tenantNamespace})
	require.NoError(t, err)

	ipamPrefixes := []struct {
		cidr      string
		namespace *string
	}{
		{cidr: "10.0.0.0/24"},
		{cidr: "10.0.3.0/24"},
		{cidr: "10.10.0.0/16"},
		{cidr: "10.0.0.0/24", namespace: tenantNamespace},
		{cidr: "10.0.2.0/24", namespace: tenantNamespace},
		// Parent prefix of the namespaced super network is copied into the tenant
		// namespace by ipam and must not be reported as missing in metal.
		{cidr: "10.10.0.0/16", namespace: tenantNamespace},
	}
	for _, p := range ipamPrefixes {
		_, err := ipam.CreatePrefix(ctx, &ipamv1.CreatePrefixRequest{
			Cidr:      p.cidr,
			Namespace: p.namespace,
		})
		require.NoError(t, err)
	}

	metalIPs := []struct {
		address   string
		namespace *string
	}{
		{address: "10.0.0.1"},
		{address: "10.0.0.2"},
		{address: "10.0.0.5", namespace: tenantNamespace},
	}
	for _, ip := range metalIPs {
		_, err := ds.IP().Create(ctx, &metal.IP{
			IPAddress:        metal.CreateNamespacedIPAddress(ip.namespace, ip.address),
			AllocationUUID:   ip.address,
			Namespace:        ip.namespace,
			ParentPrefixCidr: "10.0.0.0/24",
			Type:             metal.Ephemeral,
		})
		require.NoError(t, err)
	}

	ipamIPs := []struct {
		address   string
		namespace *string
	}{
		{address: "10.0.0.1"},
		{address: "10.0.0.3"},
		{address: "10.0.0.5", namespace: tenantNamespace},
		{address: "10.0.0.6", namespace: tenantNamespace},
	}
	for _, ip := range ipamIPs {
		_, err := ipam.AcquireIP(ctx, &ipamv1.AcquireIPRequest{
			PrefixCidr: "10.0.0.0/24",
			Ip:         &ip.address,
			Namespace:  ip.namespace,
		})
		require.NoError(t, err)
	}

	got, err := testStore.UnscopedNetwork().AdditionalMethods().NetworksConsistent(ctx)
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"missing-in-ipam"}, networkIDs(got.NetworksMissingInIpam))
	require.ElementsMatch(t, []string{"10.0.2.0/24", "10.0.3.0/24"}, networkIDs(got.NetworksMissingInMetal))

	require.Equal(t, tenantNamespace, networkNamespaceByID(got.NetworksMissingInMetal, "10.0.2.0/24"))
	require.Nil(t, networkNamespaceByID(got.NetworksMissingInMetal, "10.0.3.0/24"))

	require.ElementsMatch(t, []string{"10.0.0.2"}, ipsWithNamespace(got.IpsMissingInIpam))
	require.ElementsMatch(t, []string{"10.0.0.3", "tenant-a-10.0.0.6"}, ipsWithNamespace(got.IpsMissingInMetal))
}

func networkIDs(networks []*apiv2.Network) []string {
	var result []string
	for _, nw := range networks {
		result = append(result, nw.Id)
	}
	return result
}

func networkNamespaceByID(networks []*apiv2.Network, id string) *string {
	for _, nw := range networks {
		if nw.Id == id {
			return nw.Namespace
		}
	}
	return nil
}

func ipsWithNamespace(ips []*apiv2.IP) []string {
	var result []string
	for _, ip := range ips {
		result = append(result, metal.CreateNamespacedIPAddress(ip.Namespace, ip.Ip))
	}
	return result
}
