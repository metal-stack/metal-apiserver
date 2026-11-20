package repository_test

import (
	"fmt"
	"testing"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/stretchr/testify/require"
)

// These benchmarks measure List performance including all lookups of referenced entities
// and report the number of datastore queries per List call.
//
//	go test ./pkg/repository/ -run '^$' -bench 'Benchmark(Machine|Switch)List' -benchtime 30x
const (
	// benchMachines is the total amount of machines, created by cloning the machines of the scenario
	benchMachines = 1000
	// benchSwitchReplicas is how often the switches of the scenario are replicated
	benchSwitchReplicas = 10
)

func BenchmarkMachineList(b *testing.B) {
	dc := test.NewDatacenter(b)
	defer dc.Close()
	defer dc.Cleanup()
	dc.Create(&sc.DatacenterWithAllocations)

	ctx := b.Context()
	ds := dc.GetTestStore().GetDatastore()

	templates, err := ds.Machine().List(ctx)
	require.NoError(b, err)
	require.NotEmpty(b, templates)

	for i := range benchMachines - len(templates) {
		tpl := templates[i%len(templates)]
		m := *tpl
		m.ID = fmt.Sprintf("bench-machine-%05d", i)
		_, err := ds.Machine().Create(ctx, &m)
		require.NoError(b, err)

		if ev, err := ds.Event().Get(ctx, tpl.ID); err == nil {
			e := *ev
			e.ID = m.ID
			_, err = ds.Event().Create(ctx, &e)
			require.NoError(b, err)
		}
	}

	store, reads := newCountingStore(dc)

	reads.Store(0)
	b.ResetTimer()
	for b.Loop() {
		ms, err := store.UnscopedMachine().List(ctx, &apiv2.MachineQuery{})
		require.NoError(b, err)
		require.Len(b, ms, benchMachines)
	}

	b.ReportMetric(float64(reads.Load())/float64(b.N), "queries/op")
}

func BenchmarkSwitchList(b *testing.B) {
	dc := test.NewDatacenter(b)
	defer dc.Close()
	defer dc.Cleanup()
	dc.Create(&sc.SwitchesWithMachinesDatacenter)

	ctx := b.Context()
	ds := dc.GetTestStore().GetDatastore()

	for _, id := range []string{sc.Machine1, sc.Machine4} {
		m, err := ds.Machine().Get(ctx, id)
		require.NoError(b, err)
		m.Allocation = &metal.MachineAllocation{Project: "p1", Role: metal.RoleMachine}
		require.NoError(b, ds.Machine().Update(ctx, m))
	}

	var ips []*metal.IP

	for i := range 200 {
		ip, err := ds.IP().Create(ctx, &metal.IP{
			IPAddress:        fmt.Sprintf("192.168.%d.%d", i/250, i%250+1),
			ProjectID:        fmt.Sprintf("p%d", i%20),
			ParentPrefixCidr: "192.168.0.0/16"},
		)
		require.NoError(b, err)

		ips = append(ips, ip)
	}
	// these ips only exist in the datastore and not in ipam, so they must be removed before the datacenter cleanup releases ips in ipam
	defer func() {
		for _, ip := range ips {
			require.NoError(b, ds.IP().Delete(ctx, ip))
		}
	}()

	templates, err := ds.Switch().List(ctx)
	require.NoError(b, err)

	for r := range benchSwitchReplicas {
		if r == 0 {
			continue
		}

		for _, tpl := range templates {
			sw := *tpl
			sw.ID = fmt.Sprintf("%s-bench-%d", tpl.ID, r)
			_, err := ds.Switch().Create(ctx, &sw)
			require.NoError(b, err)
		}
	}

	total := len(templates) * benchSwitchReplicas

	store, reads := newCountingStore(dc)

	reads.Store(0)
	b.ResetTimer()
	for b.Loop() {
		sws, err := store.Switch().List(ctx, &apiv2.SwitchQuery{})
		require.NoError(b, err)
		require.Len(b, sws, total)
	}

	b.ReportMetric(float64(reads.Load())/float64(b.N), "queries/op")
}
