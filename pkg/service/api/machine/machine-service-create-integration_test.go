package machine

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func TestMachineCreateIntegration(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testToken := apiv2.Token{
		User:      "unit-test-user",
		AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
	}
	ctx = token.ContextWithToken(ctx, &testToken)

	dc := test.NewDatacenter(t, log)

	testDC := sc.DefaultDatacenter
	testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
		Name:          new("project-network"),
		ParentNetwork: new(sc.NetworkTenantSuperPartition1),
		Project:       new(sc.Tenant1Project1),
		Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD,
	})

	machineCount := 100
	allMachineUUIDs := map[string]bool{}

	for i := range machineCount {
		id, err := uuid.NewV7()
		require.NoError(t, err)
		machineID := id.String()
		allMachineUUIDs[machineID] = true

		rackId := i % 5

		testDC.Machines = append(testDC.Machines, &sc.MachineWithLiveliness{
			Machine: &metal.Machine{
				Base:        metal.Base{ID: machineID},
				PartitionID: sc.Partition1,
				RackID:      fmt.Sprintf("rack-%d", rackId),
				SizeID:      sc.SizeC1Large,
				Waiting:     true,
				Hardware: metal.MachineHardware{
					Disks: []metal.BlockDevice{
						{
							Name: "/dev/sda",
							Size: 1024 * 1024 * 1024,
						},
					},
				},
			},
			Liveliness: metal.MachineLivelinessAlive,
		})

	}

	dc.Create(&testDC)
	projectNetworkId := dc.GetNetworkByName("project-network").Id

	type teststruct struct {
		name    string
		req     *apiv2.MachineServiceCreateRequest
		want    func(dc *test.Datacenter) *apiv2.MachineServiceCreateResponse
		wantErr error
	}
	m := &machineServiceServer{
		log:  log,
		repo: dc.GetTestStore().Store,
	}

	var wg sync.WaitGroup
	for range machineCount {
		wg.Go(func() {

			req := &apiv2.MachineServiceCreateRequest{
				Name:           "testmachine",
				Project:        sc.Tenant1Project1,
				Partition:      new(sc.Partition1),
				Size:           new(sc.SizeC1Large),
				Image:          sc.ImageDebian12,
				AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
				Networks: []*apiv2.MachineAllocationNetwork{
					{Network: projectNetworkId},
				},
			}
			resp, err := m.Create(ctx, req)
			require.NoError(t, err)
			require.NotNil(t, resp)
		})
	}
	wg.Wait()

	machines, err := dc.GetTestStore().Store.Machine(sc.Tenant1Project1).List(ctx, &apiv2.MachineQuery{})
	require.NoError(t, err)

	// ensure rack spreading
	rackMap := map[string]int{}
	for _, machine := range machines {
		if machine.Allocation != nil {
			delete(allMachineUUIDs, machine.Uuid)
		}
		rackMap[machine.Rack]++
	}
	require.Len(t, rackMap, 6) // Why 6 ?
	for rack, count := range rackMap {
		require.InDelta(t, count, 19, 21, "rack is not equally loaded", rack)
	}

	require.Len(t, allMachineUUIDs, 0, "not all machines allocated")

	// Ensure all same vrf, but different asns
	var (
		asns []uint32
		vrfs []uint64
	)
	for _, machine := range machines {
		for _, network := range machine.Allocation.Networks {
			asns = append(asns, network.Asn)

			if network.NetworkType == apiv2.NetworkType_NETWORK_TYPE_UNDERLAY {
				continue
			}

			vrfs = append(vrfs, network.Vrf)
		}
	}
	require.Len(t, lo.Uniq(asns), machineCount)
	require.Len(t, lo.Uniq(vrfs), 1)

	// TODO Add firewalls
}
