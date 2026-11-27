package repository_test

import (
	"context"
	"sync/atomic"
	"testing"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/google/go-cmp/cmp"
)

// countingDatastore counts all read queries issued against the datastore.
type countingDatastore struct {
	generic.Datastore
	reads *atomic.Int64
}

type countingStorage[E generic.Entity] struct {
	generic.Storage[E]
	reads *atomic.Int64
}

func (s *countingStorage[E]) Get(ctx context.Context, id string) (E, error) {
	s.reads.Add(1)
	return s.Storage.Get(ctx, id)
}

func (s *countingStorage[E]) Find(ctx context.Context, queries ...generic.EntityQuery) (E, error) {
	s.reads.Add(1)
	return s.Storage.Find(ctx, queries...)
}

func (s *countingStorage[E]) List(ctx context.Context, queries ...generic.EntityQuery) ([]E, error) {
	s.reads.Add(1)
	return s.Storage.List(ctx, queries...)
}

func counting[E generic.Entity](s generic.Storage[E], reads *atomic.Int64) generic.Storage[E] {
	return &countingStorage[E]{Storage: s, reads: reads}
}

func (d *countingDatastore) IP() generic.Storage[*metal.IP] {
	return counting(d.Datastore.IP(), d.reads)
}
func (d *countingDatastore) Machine() generic.Storage[*metal.Machine] {
	return counting(d.Datastore.Machine(), d.reads)
}
func (d *countingDatastore) Size() generic.Storage[*metal.Size] {
	return counting(d.Datastore.Size(), d.reads)
}
func (d *countingDatastore) SizeImageConstraint() generic.Storage[*metal.SizeImageConstraint] {
	return counting(d.Datastore.SizeImageConstraint(), d.reads)
}
func (d *countingDatastore) SizeReservation() generic.Storage[*metal.SizeReservation] {
	return counting(d.Datastore.SizeReservation(), d.reads)
}
func (d *countingDatastore) Partition() generic.Storage[*metal.Partition] {
	return counting(d.Datastore.Partition(), d.reads)
}
func (d *countingDatastore) Network() generic.Storage[*metal.Network] {
	return counting(d.Datastore.Network(), d.reads)
}
func (d *countingDatastore) FilesystemLayout() generic.Storage[*metal.FilesystemLayout] {
	return counting(d.Datastore.FilesystemLayout(), d.reads)
}
func (d *countingDatastore) Image() generic.Storage[*metal.Image] {
	return counting(d.Datastore.Image(), d.reads)
}
func (d *countingDatastore) Switch() generic.Storage[*metal.Switch] {
	return counting(d.Datastore.Switch(), d.reads)
}
func (d *countingDatastore) SwitchStatus() generic.Storage[*metal.SwitchStatus] {
	return counting(d.Datastore.SwitchStatus(), d.reads)
}
func (d *countingDatastore) Event() generic.Storage[*metal.ProvisioningEventContainer] {
	return counting(d.Datastore.Event(), d.reads)
}

func newCountingStore(dc *test.Datacenter) (*repository.Store, *atomic.Int64) {
	reads := &atomic.Int64{}
	ts := dc.GetTestStore()

	return repository.New(repository.Config{
		Log:       ts.GetLogger(),
		Datastore: &countingDatastore{Datastore: ts.GetDatastore(), reads: reads},
	}), reads
}

func TestMachineList_BatchedLookups(t *testing.T) {
	t.Parallel()

	dc := test.NewDatacenter(t)
	defer dc.Close()
	dc.Create(&sc.DatacenterWithAllocations)

	ctx := t.Context()
	store, reads := newCountingStore(dc)

	machines, err := store.UnscopedMachine().List(ctx, &apiv2.MachineQuery{})
	require.NoError(t, err)
	require.Len(t, machines, len(dc.GetMachines()))
	require.NotEmpty(t, machines)

	// 1 list query + at most one query per prefetched table (partition, size, image, fsl, network, event)
	require.LessOrEqual(t, reads.Load(), int64(7), "machine list must not do lookups per machine")

	// every listed machine must be identical to the one returned by a single get
	for _, m := range machines {
		got, err := store.UnscopedMachine().Get(ctx, m.Uuid)
		require.NoError(t, err)
		if diff := cmp.Diff(got, m, protocmp.Transform()); diff != "" {
			t.Errorf("machine %s differs between list and get (-get +list):\n%s", m.Uuid, diff)
		}
	}
}

func TestSwitchList_BatchedLookups(t *testing.T) {
	t.Parallel()

	dc := test.NewDatacenter(t)
	defer dc.Close()
	dc.Create(&sc.SwitchesWithMachinesDatacenter)

	ctx := t.Context()
	ds := dc.GetTestStore().GetDatastore()

	// allocate two connected machines in different racks so that the bgp filter, ip and project machine prefetching is exercised
	for _, id := range []string{sc.Machine1, sc.Machine4} {
		m, err := ds.Machine().Get(ctx, id)
		require.NoError(t, err)
		m.Allocation = &metal.MachineAllocation{
			Project: "p1",
			Role:    metal.RoleMachine,
		}
		require.NoError(t, ds.Machine().Update(ctx, m))
	}
	for _, ip := range []string{"192.168.0.10", "192.168.0.20"} {
		_, err := ds.IP().Create(ctx, &metal.IP{
			IPAddress:        ip,
			ProjectID:        "p1",
			ParentPrefixCidr: "192.168.0.0/16"},
		)
		require.NoError(t, err)
	}

	// a firewall of the same project in the rack of machine 1 (but not of machine 4), its ip must not be announced for machine 1
	_, err := ds.Machine().Create(ctx, &metal.Machine{
		ID:          "firewall-p01-rack01",
		PartitionID: sc.Partition1,
		RackID:      sc.P01Rack01,
		Allocation: &metal.MachineAllocation{
			Project:         "p1",
			Role:            metal.RoleFirewall,
			MachineNetworks: []*metal.MachineNetwork{{IPs: []string{"192.168.0.10"}}},
		},
	})
	require.NoError(t, err)

	store, reads := newCountingStore(dc)

	switches, err := store.Switch().List(ctx, &apiv2.SwitchQuery{})
	require.NoError(t, err)
	require.Len(t, switches, len(dc.GetSwitches()))

	// list + machines, switch statuses, networks + ips, firewalls
	require.Equal(t, int64(6), reads.Load(), "switch list must not do lookups per switch, per nic or per rack")

	// collect the bgp filters of the nics connected to the allocated machines
	cidrsByMachine := map[string][]string{}
	for _, sw := range switches {
		for _, con := range sw.MachineConnections {
			if con.Nic.BgpFilter != nil && len(con.Nic.BgpFilter.Cidrs) > 0 {
				cidrsByMachine[con.MachineId] = con.Nic.BgpFilter.Cidrs
			}
		}

		got, err := store.Switch().Get(ctx, sw.Id)
		require.NoError(t, err)
		if diff := cmp.Diff(got, sw, protocmp.Transform()); diff != "" {
			t.Errorf("switch %s differs between list and get (-get +list):\n%s", sw.Id, diff)
		}
	}

	require.Equal(t, map[string][]string{
		sc.Machine1: {"192.168.0.20/32"},                    // firewall ip in the same rack is excluded
		sc.Machine4: {"192.168.0.10/32", "192.168.0.20/32"}, // firewall is in another rack
	}, cidrsByMachine)
}
