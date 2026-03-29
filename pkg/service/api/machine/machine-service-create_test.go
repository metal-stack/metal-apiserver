package machine

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_machineServiceServer_CreateMachine(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	// Add token to be able to get the user from the context
	testToken := apiv2.Token{
		User: "unit-test-user",
	}
	ctx = token.ContextWithToken(ctx, &testToken)

	dc := test.NewDatacenter(t, log)
	dc.Create(&sc.DefaultDatacenter)
	defer dc.Close()
	tests := []struct {
		name string
		req  *apiv2.MachineServiceCreateRequest
		// this func only defines the datacenter spec
		// must not be defined together with the createRequestFn
		createDatacenterFn func() *sc.DatacenterSpec
		// when this func is defined, the datacenter must be created inside
		// with the request and the expected error if any.
		// This is handy if entities with random uuids must be created as precondition
		// and also must be part of the request and the error message
		createRequestFn func() (*apiv2.MachineServiceCreateRequest, error)
		want            *apiv2.MachineServiceCreateResponse
		wantErr         error
	}{
		{
			name: "machine with private network",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:          new("project namespaced network"),
					ParentNetwork: new(sc.NetworkTenantSuperPartition1),
					Project:       new(sc.Tenant1Project1),
					Type:          apiv2.NetworkType_NETWORK_TYPE_CHILD,
				})
				testDC.Machines = append(testDC.Machines, &sc.MachineWithLiveliness{
					Machine: &metal.Machine{
						Base:        metal.Base{ID: sc.Machine5},
						PartitionID: sc.Partition1,
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
				dc.Create(&testDC)

				projectNetworkId := dc.GetNetworkByName("project namespaced network").Id
				req := &apiv2.MachineServiceCreateRequest{
					Name:           "testmachine",
					Project:        sc.Tenant1Project1,
					Partition:      sc.Partition1,
					Size:           sc.SizeC1Large,
					Image:          sc.ImageDebian12,
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: projectNetworkId},
					},
				}
				return req, nil
			},
			want: &apiv2.MachineServiceCreateResponse{
				Machine: &apiv2.Machine{
					Meta:      &apiv2.Meta{Generation: 2},
					Size:      dc.GetSizes()[sc.SizeC1Large],
					Partition: dc.GetPartitions()[sc.Partition1],
					Status: &apiv2.MachineStatus{
						Liveliness: apiv2.MachineLiveliness_MACHINE_LIVELINESS_ALIVE,
						Condition:  &apiv2.MachineCondition{},
						LedState:   &apiv2.MachineChassisIdentifyLEDState{},
					},
					Uuid: sc.Machine5,
					Hardware: &apiv2.MachineHardware{
						Disks: []*apiv2.MachineBlockDevice{
							{
								Name: "/dev/sda",
								Size: 1024 * 1024 * 1024,
							},
						},
					},
					RecentProvisioningEvents: &apiv2.MachineRecentProvisioningEvents{
						Events: []*apiv2.MachineProvisioningEvent{
							{Event: apiv2.MachineProvisioningEventType_MACHINE_PROVISIONING_EVENT_TYPE_WAITING},
						},
					},
					Allocation: &apiv2.MachineAllocation{
						Meta:     &apiv2.Meta{},
						Name:     "testmachine",
						Hostname: "metal", // Because it was not set during create
						Image:    dc.GetImages()[sc.ImageDebian12],
						Networks: []*apiv2.MachineNetwork{
							{
								// Network: dc.GetNetworkByName("project namespaced network").Id,
							},
						},
						FilesystemLayout: dc.GetFilesystemLayouts()["debian"],
						AllocationType:   apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
						CreatedBy:        "unit-test-user",
						Project:          sc.Tenant1Project1,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.createDatacenterFn != nil && tt.createRequestFn != nil {
				t.Errorf("it is not possible to define createDatacenterFn and createRequestFn")
			}
			if tt.createDatacenterFn != nil {
				dc.Cleanup()
				dc.Create(tt.createDatacenterFn())
			}
			if tt.createRequestFn != nil {
				dc.Cleanup()
				req, err := tt.createRequestFn()
				tt.req = req
				tt.wantErr = err
			}

			m := &machineServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			if tt.wantErr == nil {
				test.Validate(t, tt.req)
			}
			got, err := m.Create(ctx, tt.req)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineAllocation{}, "uuid", "networks", // FIXME networks are most important
				),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "expires_at", "url",
				),
				protocmp.IgnoreFields(
					&apiv2.PartitionBootConfiguration{}, "image_url", "kernel_url",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineProvisioningEvent{}, "time",
				),
			); diff != "" {
				t.Errorf("machineServiceServer.Create() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}
