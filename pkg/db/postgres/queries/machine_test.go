package queries_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/postgres"
	"github.com/metal-stack/metal-apiserver/pkg/db/postgres/cond"
	"github.com/metal-stack/metal-apiserver/pkg/db/postgres/queries"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tlog "github.com/testcontainers/testcontainers-go/log"
	pgmodules "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	globalPool *pgxpool.Pool
	poolMtx    sync.Mutex
)

var (
	m1 = &metal.Machine{
		Base: metal.Base{ID: "m1", Name: "m1"},
		Allocation: &metal.MachineAllocation{
			Name:    "shoot-worker-1",
			Project: "p1",
			ImageID: "debian-12",
			FilesystemLayout: &metal.FilesystemLayout{
				Base: metal.Base{ID: "c1-medium-fsl"},
				Constraints: metal.FilesystemLayoutConstraints{
					Sizes:  []string{},
					Images: map[string]string{},
				},
			},
			MachineNetworks: []*metal.MachineNetwork{},
			Hostname:        "shoot-worker-1",
			Succeeded:       true,
			Role:            metal.RoleMachine,
			UUID:            "alloc-m1",
			FirewallRules:   &metal.FirewallRules{},
			Labels:          map[string]string{"color": "red"},
		},
		PartitionID: "p1",
		SizeID:      "c1-medium",
		RackID:      "rack-1",
		RoomID:      "room-1",
		Tags:        []string{"color=red"},
		IPMI:        metal.IPMI{PowerMetric: &metal.PowerMetric{}, PowerSupplies: metal.PowerSupplies{}},
	}
	m2 = &metal.Machine{
		Base: metal.Base{ID: "m2", Name: "m2"},
		Allocation: &metal.MachineAllocation{
			Name:    "shoot-fw-m2",
			Project: "p2",
			ImageID: "firewall-ubuntu-3",
			FilesystemLayout: &metal.FilesystemLayout{
				Base: metal.Base{ID: "n1-medium-fsl"},
				Constraints: metal.FilesystemLayoutConstraints{
					Sizes:  []string{},
					Images: map[string]string{},
				},
			},
			MachineNetworks: []*metal.MachineNetwork{
				{
					NetworkID:           "internet",
					Prefixes:            []string{"1.2.3.0/24", "2.3.4.0/24"},
					IPs:                 []string{"1.2.3.4", "2.3.4.5"},
					DestinationPrefixes: []string{"0.0.0.0/0"},
					Vrf:                 104009,
					ASN:                 4009,
				},
			},
			Hostname:      "shoot-fw-m2",
			Succeeded:     true,
			Role:          metal.RoleFirewall,
			VPN:           &metal.MachineVPN{ControlPlaneAddress: "https://headscale.metal-stack.io", IPs: []string{}},
			UUID:          "alloc-m2",
			FirewallRules: &metal.FirewallRules{},
		},
		PartitionID: "p2",
		SizeID:      "n1-medium",
		RackID:      "rack-2",
		Tags:        []string{"size=medium"},
		IPMI:        metal.IPMI{PowerMetric: &metal.PowerMetric{}, PowerSupplies: metal.PowerSupplies{}},
	}
	m3 = &metal.Machine{
		Base: metal.Base{ID: "m3", Name: "m3"},
		Allocation: &metal.MachineAllocation{
			FilesystemLayout: &metal.FilesystemLayout{
				Base: metal.Base{ID: "c1-large-fsl"},
				Constraints: metal.FilesystemLayoutConstraints{
					Sizes:  []string{},
					Images: map[string]string{},
				},
			},
			MachineNetworks: []*metal.MachineNetwork{},
			FirewallRules:   &metal.FirewallRules{},
		},
		SizeID: "c1-large-x86",
		Hardware: metal.MachineHardware{
			Memory: 2048,
			Nics: metal.Nics{
				{
					MacAddress: "aa:bb",
					Name:       "eth0",
					Neighbors: metal.Nics{
						{MacAddress: "cc:dd", Name: "swp1"},
					},
				},
			},
			Disks: []metal.BlockDevice{
				{Name: "/dev/sda", Size: 4096},
			},
			MetalCPUs: []metal.MetalCPU{
				{Cores: 4},
				{Cores: 6},
			},
		},
		State: metal.MachineState{Value: metal.LockedState},
		IPMI: metal.IPMI{
			Address:    "192.168.0.1",
			MacAddress: "ee:ff",
			User:       "admin",
			Interface:  "eth1",
			Fru: metal.Fru{
				ChassisPartNumber:   "chass-1",
				ChassisPartSerial:   "chass-serial-1",
				BoardMfg:            "board-mfg-1",
				BoardMfgSerial:      "board-serial-1",
				BoardPartNumber:     "board-1",
				ProductManufacturer: "vendor-a",
				ProductPartNumber:   "vendor-a-1",
				ProductSerial:       "vendor-serial-1",
			},
			PowerMetric:   &metal.PowerMetric{},
			PowerSupplies: metal.PowerSupplies{},
		},
	}
	m4 = &metal.Machine{
		Base:        metal.Base{ID: "m4", Name: "m4"},
		PartitionID: "partition-1",
		SizeID:      "c1-xlarge",
		State:       metal.MachineState{Value: metal.AvailableState},
		Waiting:     true,
		Tags:        []string{},
		IPMI:        metal.IPMI{PowerMetric: &metal.PowerMetric{}, PowerSupplies: metal.PowerSupplies{}},
	}
	machines = []*metal.Machine{m1, m2, m3, m4}
)

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}

func setup() {
	poolMtx.Lock()
	defer poolMtx.Unlock()

	if globalPool != nil {
		return
	}

	ctx := context.Background()

	c, err := pgmodules.Run(
		ctx,
		"postgres:18-alpine",
		pgmodules.WithPassword("password"),
		testcontainers.WithTmpfs(map[string]string{"/var/lib/postgresql/data": "rw"}),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(30*time.Second),
			wait.ForLog("database system is ready to accept connections"),
		),
		testcontainers.WithLogger(tlog.TestLogger(&testing.T{})),
	)
	if err != nil {
		panic(fmt.Sprintf("cannot start postgres container: %v", err))
	}

	connStr, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(fmt.Sprintf("cannot get connection string: %v", err))
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		panic(fmt.Sprintf("cannot create pool: %v", err))
	}

	globalPool = pool
}

func teardown() {
	if globalPool != nil {
		globalPool.Close()
	}
}

// withTestDB sets up the machine table in the global test database,
// truncates any existing data, inserts the test machines, and returns the pool.
func withTestDB(t testing.TB) *pgxpool.Pool {
	t.Helper()

	ctx := t.Context()

	// Ensure the table exists
	_, err := globalPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS machine (
			id TEXT PRIMARY KEY,
			data JSONB NOT NULL,
			created TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			changed TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			generation BIGINT NOT NULL DEFAULT 0
		)
	`)
	require.NoError(t, err)

	// Clean any previous test data
	_, err = globalPool.Exec(ctx, `TRUNCATE machine`)
	require.NoError(t, err)

	for _, m := range machines {
		data, err := json.Marshal(m)
		require.NoError(t, err)
		_, err = globalPool.Exec(ctx,
			`INSERT INTO machine (id, data, created, changed, generation) VALUES ($1, $2, $3, $4, $5)`,
			m.ID, data, time.Now(), time.Now(), 0,
		)
		require.NoError(t, err)
	}

	// Debug
	var count int
	globalPool.QueryRow(ctx, `SELECT count(*) FROM machine`).Scan(&count)
	fmt.Printf("DEBUG withTestDB machine count after insert: %d\n", count)

	return globalPool
}

// runFilter executes a machine query against the given pool and returns matching machines.
func runFilter(ctx context.Context, pool *pgxpool.Pool, filter *cond.Where) ([]*metal.Machine, error) {
	var conds []*cond.Where
	if filter != nil {
		conds = []*cond.Where{filter}
	}
	whereSQL, whereArgs := postgres.BuildWhereClause(conds, 1)
	query := `SELECT id, data FROM "machine"` + whereSQL

	rows, err := pool.Query(ctx, query, whereArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*metal.Machine
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("cannot scan: %w", err)
		}
		var m metal.Machine
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		m.Created = time.Time{}
		m.Changed = time.Time{}
		results = append(results, &m)
	}
	return results, nil
}

func TestMachineFilter(t *testing.T) {
	pool := withTestDB(t)
	ctx := t.Context()

	tests := []struct {
		name string
		rq   *apiv2.MachineQuery
		want []*metal.Machine
	}{
		{
			name: "empty request returns unfiltered",
			rq:   nil,
			want: []*metal.Machine{m1, m2, m3, m4},
		},
		{
			name: "by id",
			rq:   &apiv2.MachineQuery{Uuid: &m1.ID},
			want: []*metal.Machine{m1},
		},
		{
			name: "by id 2",
			rq:   &apiv2.MachineQuery{Uuid: &m2.ID},
			want: []*metal.Machine{m2},
		},
		{
			name: "by allocation uuid 1",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Uuid: new("alloc-m1")}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by name",
			rq:   &apiv2.MachineQuery{Name: &m1.Name},
			want: []*metal.Machine{m1},
		},
		{
			name: "by label",
			rq:   &apiv2.MachineQuery{Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by label 2",
			rq:   &apiv2.MachineQuery{Labels: &apiv2.Labels{Labels: map[string]string{"size": "medium"}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by partition",
			rq:   &apiv2.MachineQuery{Partition: &m1.PartitionID},
			want: []*metal.Machine{m1},
		},
		{
			name: "by size",
			rq:   &apiv2.MachineQuery{Size: new("n1-medium")},
			want: []*metal.Machine{m2},
		},
		{
			name: "by rack",
			rq:   &apiv2.MachineQuery{Rack: new("rack-2")},
			want: []*metal.Machine{m2},
		},
		{
			name: "by room",
			rq:   &apiv2.MachineQuery{Room: new("room-1")},
			want: []*metal.Machine{m1},
		},
		{
			name: "by state",
			rq:   &apiv2.MachineQuery{State: apiv2.MachineState_MACHINE_STATE_LOCKED.Enum()},
			want: []*metal.Machine{m3},
		},
		{
			name: "by allocation name",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Name: new("shoot-worker-1")}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by allocation hostname",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Hostname: new("shoot-worker-1")}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by allocation project",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Project: new("p1")}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by allocation role machine",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE.Enum()}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by allocation role firewall",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL.Enum()}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by allocation image",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Image: new("debian-12")}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by allocation fsl",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{FilesystemLayout: new("n1-medium-fsl")}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by allocation labels",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}}},
			want: []*metal.Machine{m1},
		},
		{
			name: "by allocation vpn",
			rq:   &apiv2.MachineQuery{Allocation: &apiv2.MachineAllocationQuery{Vpn: &apiv2.MachineVPN{}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by network id",
			rq:   &apiv2.MachineQuery{Network: &apiv2.MachineNetworkQuery{Networks: []string{"internet"}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by network prefixes",
			rq:   &apiv2.MachineQuery{Network: &apiv2.MachineNetworkQuery{Prefixes: []string{"1.2.3.0/24"}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by network destinationprefixes",
			rq:   &apiv2.MachineQuery{Network: &apiv2.MachineNetworkQuery{DestinationPrefixes: []string{"0.0.0.0/0"}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by network ips",
			rq:   &apiv2.MachineQuery{Network: &apiv2.MachineNetworkQuery{Ips: []string{"1.2.3.4"}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by network vrf",
			rq:   &apiv2.MachineQuery{Network: &apiv2.MachineNetworkQuery{Vrfs: []uint64{104009}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by network asn",
			rq:   &apiv2.MachineQuery{Network: &apiv2.MachineNetworkQuery{Asns: []uint32{4009}}},
			want: []*metal.Machine{m2},
		},
		{
			name: "by hardware memory",
			rq:   &apiv2.MachineQuery{Hardware: &apiv2.MachineHardwareQuery{Memory: new(uint64(2048))}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by hardware cpus",
			rq:   &apiv2.MachineQuery{Hardware: &apiv2.MachineHardwareQuery{CpuCores: new(uint32(10))}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by nic mac",
			rq:   &apiv2.MachineQuery{Nic: &apiv2.MachineNicQuery{Macs: []string{"aa:bb"}}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by nic name",
			rq:   &apiv2.MachineQuery{Nic: &apiv2.MachineNicQuery{Names: []string{"eth0"}}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by nic neighbor mac",
			rq:   &apiv2.MachineQuery{Nic: &apiv2.MachineNicQuery{NeighborMacs: []string{"cc:dd"}}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by nic neighbor name",
			rq:   &apiv2.MachineQuery{Nic: &apiv2.MachineNicQuery{NeighborNames: []string{"swp1"}}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by disk name",
			rq:   &apiv2.MachineQuery{Disk: &apiv2.MachineDiskQuery{Names: []string{"/dev/sda"}}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by disk size",
			rq:   &apiv2.MachineQuery{Disk: &apiv2.MachineDiskQuery{Sizes: []uint64{4096}}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by ipmi address",
			rq:   &apiv2.MachineQuery{Bmc: &apiv2.MachineBMCQuery{Address: new("192.168.0.1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by ipmi mac",
			rq:   &apiv2.MachineQuery{Bmc: &apiv2.MachineBMCQuery{Mac: new("ee:ff")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by ipmi user",
			rq:   &apiv2.MachineQuery{Bmc: &apiv2.MachineBMCQuery{User: new("admin")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by ipmi interface",
			rq:   &apiv2.MachineQuery{Bmc: &apiv2.MachineBMCQuery{Interface: new("eth1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru chassispartnumber",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{ChassisPartNumber: new("chass-1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru chassispartserial",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{ChassisPartSerial: new("chass-serial-1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru boardmfg",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{BoardMfg: new("board-mfg-1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru boardserial",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{BoardSerial: new("board-serial-1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru boardpartnumber",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{BoardPartNumber: new("board-1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru productmanufacturer",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{ProductManufacturer: new("vendor-a")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru productpartnumber",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{ProductPartNumber: new("vendor-a-1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "by fru productserial",
			rq:   &apiv2.MachineQuery{Fru: &apiv2.MachineFRUQuery{ProductSerial: new("vendor-serial-1")}},
			want: []*metal.Machine{m3},
		},
		{
			name: "find a waiting machine not allocated nor preallocate and available",
			rq: &apiv2.MachineQuery{
				Partition:    new("partition-1"),
				Size:         new("c1-xlarge"),
				State:        apiv2.MachineState_MACHINE_STATE_AVAILABLE.Enum(),
				Waiting:      new(true),
				Preallocated: new(false),
				NotAllocated: new(true),
			},
			want: []*metal.Machine{m4},
		},
		{
			name: "find a not allocated machine",
			rq: &apiv2.MachineQuery{
				NotAllocated: new(true),
			},
			want: []*metal.Machine{m4},
		},
		{
			name: "find allocated machine",
			rq: &apiv2.MachineQuery{
				Allocation: &apiv2.MachineAllocationQuery{},
			},
			want: []*metal.Machine{m1, m2, m3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := queries.MachineFilter(tt.rq)
			got, err := runFilter(ctx, pool, filter)
			require.NoError(t, err)

			slices.SortFunc(got, func(a, b *metal.Machine) int {
				return strings.Compare(a.ID, b.ID)
			})

			gotIDs := make([]string, len(got))
			for i, m := range got {
				gotIDs[i] = m.ID
			}
			wantIDs := make([]string, len(tt.want))
			for i, m := range tt.want {
				wantIDs[i] = m.ID
			}

			require.Equal(t, wantIDs, gotIDs, "machine IDs mismatch")
		})
	}
}
