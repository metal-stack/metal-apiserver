package machine

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMachineCreate_Rollback(t *testing.T) {
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

	uid := uuid.New()
	testDC.Machines = append(testDC.Machines, &sc.MachineWithLiveliness{
		Machine: &metal.Machine{
			Base:        metal.Base{ID: uid.String()},
			PartitionID: sc.Partition1,
			RackID:      "rack-01",
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
	testDC.IPs = append(testDC.IPs, &apiv2.IPServiceCreateRequest{
		Network: sc.NetworkInternet,
		Project: sc.Tenant1Project1,
		Name:    new("my internet service"),
		Type:    apiv2.IPType_IP_TYPE_STATIC.Enum(),
		Ip:      new("1.2.3.42"),
	})

	dc.Create(&testDC)
	projectNetworkId := dc.GetNetworkByName("project-network").Id

	m := &machineServiceServer{
		log:  log,
		repo: dc.GetTestStore().Store,
	}
	req := &apiv2.MachineServiceCreateRequest{
		Name:           "testmachine",
		Project:        sc.Tenant1Project1,
		Partition:      new(sc.Partition1),
		Size:           new(sc.SizeC1Large),
		Image:          sc.ImageDebian12,
		AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
		Networks: []*apiv2.MachineAllocationNetwork{
			{Network: sc.NetworkInternet, Ips: []string{"1.2.3.42"}},
			{Network: projectNetworkId},
		},
	}

	machinesBefore, err := dc.GetTestStore().Store.Machine(sc.Tenant1Project1).List(ctx, &apiv2.MachineQuery{})
	require.NoError(t, err)

	ipsBefore, err := dc.GetTestStore().Store.UnscopedIP().List(ctx, &apiv2.IPQuery{})
	require.NoError(t, err)

	// Inject failing rethinkdb right before storing the machine with the allocation
	ctx = context.WithValue(ctx, repository.InjectRethinkDbError("true"), "rethinkdb error injected")
	require.NoError(t, err)

	resp, err := m.Create(ctx, req)
	require.EqualError(t, err, errorutil.Internal("injected error:rethinkdb error injected").Error())
	require.Nil(t, resp)

	machines, err := dc.GetTestStore().Store.Machine(sc.Tenant1Project1).List(ctx, &apiv2.MachineQuery{})
	require.NoError(t, err)
	// Same amount of machines as before
	require.Len(t, machines, len(machinesBefore))

	// Let the ip delete task do its work
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ips, err := dc.GetTestStore().Store.UnscopedIP().List(ctx, &apiv2.IPQuery{})
		require.NoError(c, err)
		// Same amount of ips as before
		require.Len(c, ips, len(ipsBefore))
	}, 5*time.Second, 100*time.Millisecond)

	// TODO check ASN is released

}
