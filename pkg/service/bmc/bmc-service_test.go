package bmc

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	m0 = "00000000-0000-0000-0000-000000000000"
	m1 = "00000000-0000-0000-0000-000000000001"

	p1 = "00000000-0000-0000-0000-000000000001"
	p2 = "00000000-0000-0000-0000-000000000002"
)

func Test_bmcServiceServer_UpdateBMCInfo(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	ctx := t.Context()

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, repo, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})
	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})
	test.CreateSizes(t, repo, []*adminv2.SizeServiceCreateRequest{
		{
			Size: &apiv2.Size{Id: "c1-large-x86"},
		},
	})
	test.CreateImages(t, repo, []*adminv2.ImageServiceCreateRequest{
		{Image: &apiv2.Image{Id: "debian-12", Url: validURL, Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE}}},
	})

	// We need to create machines directly on the database because there is no MachineCreateRequest available and never will.
	test.CreateMachines(t, testStore, []*metal.Machine{
		{Base: metal.Base{ID: m1}, PartitionID: "partition-1", SizeID: "c1-large-x86"},
	})

	tests := []struct {
		name    string
		req     *infrav2.UpdateBMCInfoRequest
		want    *adminv2.MachineServiceListBMCResponse
		wantErr error
	}{
		{
			name: "update bmc info for unknown machine, no values",
			req: &infrav2.UpdateBMCInfoRequest{Partition: "partition-1", BmcReports: map[string]*apiv2.MachineBMCReport{
				m0: {
					Bmc:           &apiv2.MachineBMC{},
					Bios:          &apiv2.MachineBios{},
					Fru:           &apiv2.MachineFRU{},
					PowerMetric:   &apiv2.MachinePowerMetric{},
					PowerSupplies: []*apiv2.MachinePowerSupply{},
					LedState:      &apiv2.MachineChassisIdentifyLEDState{Value: "LED-OFF"},
				},
			}},
			want: &adminv2.MachineServiceListBMCResponse{
				BmcReports: map[string]*apiv2.MachineBMCReport{
					m0: {
						Bmc:  &apiv2.MachineBMC{},
						Bios: &apiv2.MachineBios{},
						Fru: &apiv2.MachineFRU{
							ChassisPartNumber:   pointer.Pointer(""),
							ChassisPartSerial:   pointer.Pointer(""),
							BoardMfg:            pointer.Pointer(""),
							BoardMfgSerial:      pointer.Pointer(""),
							BoardPartNumber:     pointer.Pointer(""),
							ProductManufacturer: pointer.Pointer(""),
							ProductPartNumber:   pointer.Pointer(""),
							ProductSerial:       pointer.Pointer(""),
						},
						PowerSupplies: []*apiv2.MachinePowerSupply{},
						LedState:      &apiv2.MachineChassisIdentifyLEDState{Value: "LED-OFF"},
					},
					m1: {
						Bmc:  &apiv2.MachineBMC{},
						Bios: &apiv2.MachineBios{},
						Fru: &apiv2.MachineFRU{
							ChassisPartNumber:   pointer.Pointer(""),
							ChassisPartSerial:   pointer.Pointer(""),
							BoardMfg:            pointer.Pointer(""),
							BoardMfgSerial:      pointer.Pointer(""),
							BoardPartNumber:     pointer.Pointer(""),
							ProductManufacturer: pointer.Pointer(""),
							ProductPartNumber:   pointer.Pointer(""),
							ProductSerial:       pointer.Pointer(""),
						},
						PowerSupplies: []*apiv2.MachinePowerSupply{},
						LedState:      &apiv2.MachineChassisIdentifyLEDState{},
					},
				}},
			wantErr: nil,
		},
		{
			name: "update bmc info for known machine, no values",
			req: &infrav2.UpdateBMCInfoRequest{Partition: "partition-1", BmcReports: map[string]*apiv2.MachineBMCReport{
				m1: {
					Bmc:           &apiv2.MachineBMC{},
					Bios:          &apiv2.MachineBios{},
					Fru:           &apiv2.MachineFRU{},
					PowerMetric:   &apiv2.MachinePowerMetric{},
					PowerSupplies: []*apiv2.MachinePowerSupply{},
					LedState:      &apiv2.MachineChassisIdentifyLEDState{Value: "LED-OFF"},
				},
			}},
			want: &adminv2.MachineServiceListBMCResponse{
				BmcReports: map[string]*apiv2.MachineBMCReport{
					m0: {
						Bmc:  &apiv2.MachineBMC{},
						Bios: &apiv2.MachineBios{},
						Fru: &apiv2.MachineFRU{
							ChassisPartNumber:   pointer.Pointer(""),
							ChassisPartSerial:   pointer.Pointer(""),
							BoardMfg:            pointer.Pointer(""),
							BoardMfgSerial:      pointer.Pointer(""),
							BoardPartNumber:     pointer.Pointer(""),
							ProductManufacturer: pointer.Pointer(""),
							ProductPartNumber:   pointer.Pointer(""),
							ProductSerial:       pointer.Pointer(""),
						},
						PowerSupplies: []*apiv2.MachinePowerSupply{},
						LedState:      &apiv2.MachineChassisIdentifyLEDState{Value: "LED-OFF"}},
					m1: {
						Bmc:  &apiv2.MachineBMC{},
						Bios: &apiv2.MachineBios{},
						Fru: &apiv2.MachineFRU{
							ChassisPartNumber:   pointer.Pointer(""),
							ChassisPartSerial:   pointer.Pointer(""),
							BoardMfg:            pointer.Pointer(""),
							BoardMfgSerial:      pointer.Pointer(""),
							BoardPartNumber:     pointer.Pointer(""),
							ProductManufacturer: pointer.Pointer(""),
							ProductPartNumber:   pointer.Pointer(""),
							ProductSerial:       pointer.Pointer(""),
						},
						PowerMetric:   &apiv2.MachinePowerMetric{},
						PowerSupplies: []*apiv2.MachinePowerSupply{},
						LedState:      &apiv2.MachineChassisIdentifyLEDState{Value: "LED-OFF"},
					},
				}},
			wantErr: nil,
		},

		{
			name: "update bmc info for known machine, all values specified",
			req: &infrav2.UpdateBMCInfoRequest{Partition: "partition-1", BmcReports: map[string]*apiv2.MachineBMCReport{
				m1: {
					Bmc: &apiv2.MachineBMC{
						Address:    "1.2.3.4:631",
						Mac:        "00:00:00:00:00:01",
						User:       "metal",
						Password:   "secret",
						Interface:  "eth0",
						Version:    "1.0.1",
						PowerState: "ON",
					},
					Bios: &apiv2.MachineBios{
						Version: "2.0.1",
						Vendor:  "SMC",
						Date:    "2025-01-01",
					},
					Fru: &apiv2.MachineFRU{
						ChassisPartNumber:   pointer.Pointer("cpn-1"),
						ChassisPartSerial:   pointer.Pointer("cps-2"),
						BoardMfg:            pointer.Pointer("bmfg-3"),
						BoardMfgSerial:      pointer.Pointer("bmfgserial-4"),
						BoardPartNumber:     pointer.Pointer("bpn-5"),
						ProductManufacturer: pointer.Pointer("pm-6"),
						ProductPartNumber:   pointer.Pointer("ppn-7"),
						ProductSerial:       pointer.Pointer("ps-8"),
					},
					PowerMetric: &apiv2.MachinePowerMetric{
						AverageConsumedWatts: 12.3,
						IntervalInMin:        5.0,
						MaxConsumedWatts:     15.4,
						MinConsumedWatts:     11.9,
					},
					PowerSupplies: []*apiv2.MachinePowerSupply{
						{Health: "OK", State: "ON"},
					},
					LedState: &apiv2.MachineChassisIdentifyLEDState{Value: "LED-OFF"},
				},
			}},
			want: &adminv2.MachineServiceListBMCResponse{
				BmcReports: map[string]*apiv2.MachineBMCReport{
					m0: {
						Bmc:  &apiv2.MachineBMC{},
						Bios: &apiv2.MachineBios{},
						Fru: &apiv2.MachineFRU{
							ChassisPartNumber:   pointer.Pointer(""),
							ChassisPartSerial:   pointer.Pointer(""),
							BoardMfg:            pointer.Pointer(""),
							BoardMfgSerial:      pointer.Pointer(""),
							BoardPartNumber:     pointer.Pointer(""),
							ProductManufacturer: pointer.Pointer(""),
							ProductPartNumber:   pointer.Pointer(""),
							ProductSerial:       pointer.Pointer(""),
						},
						PowerSupplies: []*apiv2.MachinePowerSupply{},
						LedState:      &apiv2.MachineChassisIdentifyLEDState{Value: "LED-OFF"}},
					m1: {
						Bmc: &apiv2.MachineBMC{
							Address:    "1.2.3.4:631",
							Mac:        "00:00:00:00:00:01",
							User:       "metal",
							Password:   "secret",
							Interface:  "eth0",
							Version:    "1.0.1",
							PowerState: "ON",
						},
						Bios: &apiv2.MachineBios{
							Version: "2.0.1",
							Vendor:  "SMC",
							Date:    "2025-01-01",
						},
						Fru: &apiv2.MachineFRU{
							ChassisPartNumber:   pointer.Pointer("cpn-1"),
							ChassisPartSerial:   pointer.Pointer("cps-2"),
							BoardMfg:            pointer.Pointer("bmfg-3"),
							BoardMfgSerial:      pointer.Pointer("bmfgserial-4"),
							BoardPartNumber:     pointer.Pointer("bpn-5"),
							ProductManufacturer: pointer.Pointer("pm-6"),
							ProductPartNumber:   pointer.Pointer("ppn-7"),
							ProductSerial:       pointer.Pointer("ps-8"),
						},
						PowerMetric: &apiv2.MachinePowerMetric{
							AverageConsumedWatts: 12.3,
							IntervalInMin:        5.0,
							MaxConsumedWatts:     15.4,
							MinConsumedWatts:     11.9,
						},
						PowerSupplies: []*apiv2.MachinePowerSupply{
							{Health: "OK", State: "ON"},
						},
						LedState: &apiv2.MachineChassisIdentifyLEDState{Value: "LED-OFF"},
					},
				}},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bmcServiceServer{
				log:  log,
				repo: repo,
			}
			_, err := b.UpdateBMCInfo(ctx, tt.req)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("bmcServiceServer.UpdateBMCInfo() error diff = %s", diff)
			}
			got, err := repo.UnscopedMachine().AdditionalMethods().ListBMC(ctx, &adminv2.MachineServiceListBMCRequest{})
			require.NoError(t, err)

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineBMCReport{}, "updated_at",
				),
			); diff != "" {
				t.Errorf("bmcServiceServer.UpdateBMCInfo()  diff = %s", diff)
			}
		})
	}
}
