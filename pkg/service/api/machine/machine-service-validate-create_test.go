package machine

import (
	"context"
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
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_machineServiceServer_ValidateCreateMachine(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Add token to be able to get the user from the context
	testToken := apiv2.Token{
		User:      "unit-test-user",
		AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
	}

	dc := test.NewDatacenter(t, log)
	dc.Create(&sc.DefaultDatacenter)
	defer dc.Close()

	tests := []struct {
		name  string
		req   *apiv2.MachineServiceCreateRequest
		ctxFn func() context.Context
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
			name:    "no token given",
			req:     &apiv2.MachineServiceCreateRequest{},
			ctxFn:   func() context.Context { return context.Background() },
			want:    nil,
			wantErr: errorutil.Unauthenticated("unable to get user from context"),
		},
		{
			name:    "no project given",
			req:     &apiv2.MachineServiceCreateRequest{},
			want:    nil,
			wantErr: errorutil.NotFound("get of project with id "),
		},
		{
			name:    "project does not exist",
			req:     &apiv2.MachineServiceCreateRequest{Project: "abc"},
			want:    nil,
			wantErr: errorutil.NotFound("get of project with id abc"),
		},
		{
			name:    "partition does not exist",
			req:     &apiv2.MachineServiceCreateRequest{Project: sc.Tenant1Project1, Partition: new("non-existing-partition"), Image: sc.ImageDebian12},
			want:    nil,
			wantErr: errorutil.NotFound(`no partition with id "non-existing-partition" found`),
		},
		{
			name:    "size does not exist",
			req:     &apiv2.MachineServiceCreateRequest{Project: sc.Tenant1Project1, Partition: new(sc.Partition1), Size: new("unknown-size"), Image: sc.ImageDebian12},
			want:    nil,
			wantErr: errorutil.NotFound(`no size with id "unknown-size" found`),
		},
		// UUID is specified
		{
			name: "uuid is specified, but user is not admin",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:      new(sc.Machine1),
				Project:   sc.Tenant1Project1,
				Partition: new(sc.Partition1),
			},
			ctxFn: func() context.Context {
				testToken := apiv2.Token{
					User: "unit-test-user",
				}
				return token.ContextWithToken(context.Background(), &testToken)
			},
			want:    nil,
			wantErr: errorutil.Unauthenticated("only admins can create machines with a specific uuid"),
		},
		{
			name: "uuid is specified, but partition is given",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:      new(sc.Machine1),
				Project:   sc.Tenant1Project1,
				Partition: new(sc.Partition1),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("when machine id is given, a partition must not be specified"),
		},
		{
			name: "uuid is specified, but size is given",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:    new(sc.Machine1),
				Project: sc.Tenant1Project1,
				Size:    new(sc.SizeC1Large),
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("when machine id is given, a size must not be specified"),
		},
		{
			name: "uuid is specified, but machine is allocated",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:    new(sc.Machine1),
				Project: sc.Tenant1Project1,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("machine 00000000-0000-0000-0000-000000000001 is already allocated"),
		},
		{
			name: "uuid is specified, but machine is locked",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:    new(sc.Machine2),
				Project: sc.Tenant1Project1,
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				testDC := sc.DefaultDatacenter
				testDC.Machines = []*sc.MachineWithLiveliness{
					sc.MachineFunc(sc.Machine2, sc.Partition1, sc.SizeN1Medium, "", "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[0].Machine.State.Value = metal.LockedState
				return &testDC
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("machine 00000000-0000-0000-0000-000000000002 is LOCKED"),
		},
		{
			name: "uuid is specified, but machine is not waiting",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:    new(sc.Machine2),
				Project: sc.Tenant1Project1,
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				testDC := sc.DefaultDatacenter
				testDC.Machines = []*sc.MachineWithLiveliness{
					sc.MachineFunc(sc.Machine2, sc.Partition1, sc.SizeN1Medium, "", "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[0].Machine.Waiting = false
				return &testDC
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("machine 00000000-0000-0000-0000-000000000002 is not waiting"),
		},
		// UUID is not specified
		{
			name: "partition is not present",
			req: &apiv2.MachineServiceCreateRequest{
				Project:   sc.Tenant1Project1,
				Partition: new(sc.Partition2),
				Image:     sc.ImageDebian12,
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no partition with id "partition-2" found`),
		},
		{
			name: "size is not present",
			req: &apiv2.MachineServiceCreateRequest{
				Project:   sc.Tenant1Project1,
				Partition: new(sc.Partition1),
				Image:     sc.ImageDebian12,
				Size:      new("unknown-size"),
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no size with id "unknown-size" found`),
		},
		{
			name: "image is not present",
			req: &apiv2.MachineServiceCreateRequest{
				Project:   sc.Tenant1Project1,
				Partition: new(sc.Partition1),
				Size:      new(sc.SizeC1Large),
				Image:     "unknown-11",
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no image for os:unknown version:11.0.0 found`),
		},
		{
			name: "fsl is given but does not exists",
			req: &apiv2.MachineServiceCreateRequest{
				Project:          sc.Tenant1Project1,
				Partition:        new(sc.Partition1),
				Size:             new(sc.SizeC1Large),
				FilesystemLayout: new("debian-fsl"),
				Image:            "debian-13",
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no filesystemlayout with id "debian-fsl" found`),
		},
		{
			name: "uuid and fsl is given but does not match hardware",
			req: &apiv2.MachineServiceCreateRequest{
				Uuid:             new(sc.Machine1),
				Project:          sc.Tenant1Project1,
				FilesystemLayout: new("debian-13"),
				Image:            "debian-13",
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				testDC := sc.DefaultDatacenter
				testDC.Machines = []*sc.MachineWithLiveliness{
					sc.MachineFunc(sc.Machine1, sc.Partition1, sc.SizeN1Medium, "", "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[0].Machine.Waiting = true
				testDC.Machines[0].Machine.Hardware = metal.MachineHardware{
					Disks: []metal.BlockDevice{
						{Name: "/dev/sdb"},
					},
				}
				testDC.FilesystemLayouts = []*adminv2.FilesystemServiceCreateRequest{
					{
						FilesystemLayout: &apiv2.FilesystemLayout{
							Id: "debian-13",
							Constraints: &apiv2.FilesystemLayoutConstraints{
								Sizes: []string{sc.SizeN1Medium},
								Images: map[string]string{
									"debian": ">= 12.0",
								},
							},
							Disks: []*apiv2.Disk{
								{
									Device: "/dev/sda",
								},
							},
						},
					},
				}

				return &testDC
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`device:/dev/sda does not exist on given hardware`),
		},
		{
			name: "no fsl is given and no matching one found",
			req: &apiv2.MachineServiceCreateRequest{
				Project:   sc.Tenant1Project1,
				Partition: new(sc.Partition1),
				Size:      new(sc.SizeC1Large),
				Image:     "debian-11",
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`could not find a matching filesystemLayout for size:c1-large-x86 and image:debian-11.0.20241220`),
		},
		{
			name: "no fsl is given but present, but no match for image and size",
			req: &apiv2.MachineServiceCreateRequest{
				Project:   sc.Tenant1Project1,
				Partition: new(sc.Partition1),
				Size:      new(sc.SizeC1Large),
				Image:     "debian-11",
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				return &sc.DefaultDatacenter
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`could not find a matching filesystemLayout for size:c1-large-x86 and image:debian-11.0.20241220`),
		},
		// Wrong Allocation Types
		{
			name: "allocation type wrong",
			req: &apiv2.MachineServiceCreateRequest{
				Project:        sc.Tenant1Project1,
				Partition:      new(sc.Partition1),
				Size:           new(sc.SizeC1Large),
				Image:          "debian-13",
				AllocationType: 0,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given allocationtype MACHINE_ALLOCATION_TYPE_UNSPECIFIED is not supported`),
		},
		{
			name: "image type wrong",
			req: &apiv2.MachineServiceCreateRequest{
				Project:        sc.Tenant1Project1,
				Partition:      new(sc.Partition1),
				Size:           new(sc.SizeN1Medium),
				Image:          sc.ImageFirewall3_0,
				AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given image %s is not allowed for machines`, sc.ImageFirewall3_0),
		},
		{
			name: "machine with firewall rules",
			req: &apiv2.MachineServiceCreateRequest{
				Project:        sc.Tenant1Project1,
				Partition:      new(sc.Partition1),
				Size:           new(sc.SizeC1Large),
				Image:          "debian-13",
				AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
				FirewallSpec:   &apiv2.FirewallSpec{},
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				return &sc.DefaultDatacenter
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`firewall rules can only be specified on firewalls`),
		},
		// Networks
		{
			name: "machine with no networks",
			req: &apiv2.MachineServiceCreateRequest{
				Project:        sc.Tenant1Project1,
				Partition:      new(sc.Partition1),
				Size:           new(sc.SizeC1Large),
				Image:          "debian-13",
				AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				return &sc.DefaultDatacenter
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`networks must not be empty`),
		},
		{
			name: "machine with unknown networks",
			req: &apiv2.MachineServiceCreateRequest{
				Project:        sc.Tenant1Project1,
				Partition:      new(sc.Partition1),
				Size:           new(sc.SizeC1Large),
				Image:          "debian-13",
				AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
				Networks: []*apiv2.MachineAllocationNetwork{
					{Network: "no-internet"},
				},
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				return &sc.DefaultDatacenter
			},
			want:    nil,
			wantErr: errorutil.NotFound(`no network with id "no-internet" found`),
		},
		{
			name: "machine with duplicate networks",
			req: &apiv2.MachineServiceCreateRequest{
				Project:        sc.Tenant1Project1,
				Partition:      new(sc.Partition1),
				Size:           new(sc.SizeC1Large),
				Image:          "debian-13",
				AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
				Networks: []*apiv2.MachineAllocationNetwork{
					{Network: "internet"},
					{Network: "internet"},
				},
			},
			createDatacenterFn: func() *sc.DatacenterSpec {
				return &sc.DefaultDatacenter
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given network ids are not unique`),
		},
		{
			name: "machine with private network in wrong partition",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.Partitions = []string{sc.Partition1, sc.Partition2}
				testDC.ProjectsPerTenant = 2
				testDC.FilesystemLayouts = []*adminv2.FilesystemServiceCreateRequest{
					{
						FilesystemLayout: &apiv2.FilesystemLayout{
							Id: "debian",
							Constraints: &apiv2.FilesystemLayoutConstraints{
								Sizes: []string{sc.SizeC1Large},
								Images: map[string]string{
									"debian": ">= 12.0",
								},
							},
						},
					},
				}
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:      new("project network"),
					Project:   new(sc.Tenant1Project1),
					Partition: new(sc.Partition1),
					Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				})
				dc.Create(&testDC)

				projectNetworkId := dc.GetNetworkByName("project network").Id
				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project1,
					Partition:      new(sc.Partition2),
					Size:           new(sc.SizeC1Large),
					Image:          "debian-13",
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet},
						{Network: projectNetworkId},
					},
				}
				return req, errorutil.InvalidArgument(`network %q must be located in the partition where the machine is going to be placed`, projectNetworkId)
			},
			want: nil,
		},
		{
			name: "machine with supernetwork",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.Partitions = []string{sc.Partition1, sc.Partition2}
				testDC.ProjectsPerTenant = 2
				testDC.FilesystemLayouts = []*adminv2.FilesystemServiceCreateRequest{
					{
						FilesystemLayout: &apiv2.FilesystemLayout{
							Id: "debian",
							Constraints: &apiv2.FilesystemLayoutConstraints{
								Sizes: []string{sc.SizeC1Large},
								Images: map[string]string{
									"debian": ">= 12.0",
								},
							},
						},
					},
				}
				dc.Create(&testDC)

				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project1,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeC1Large),
					Image:          "debian-13",
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet},
						{Network: sc.NetworkTenantSuperPartition1},
					},
				}
				return req, errorutil.InvalidArgument(`super networks can not be specified as allocation networks`)
			},
			want: nil,
		},

		{
			name: "machine with two private networks",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.FilesystemLayouts = []*adminv2.FilesystemServiceCreateRequest{
					{
						FilesystemLayout: &apiv2.FilesystemLayout{
							Id: "debian",
							Constraints: &apiv2.FilesystemLayoutConstraints{
								Sizes: []string{sc.SizeC1Large},
								Images: map[string]string{
									"debian": ">= 12.0",
								},
							},
						},
					},
				}
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:      new("project network 1"),
					Project:   new(sc.Tenant1Project1),
					Partition: new(sc.Partition1),
					Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				},
					&adminv2.NetworkServiceCreateRequest{
						Name:      new("project network 2"),
						Project:   new(sc.Tenant1Project1),
						Partition: new(sc.Partition1),
						Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
					},
				)
				dc.Create(&testDC)

				projectNetwork1Id := dc.GetNetworkByName("project network 1").Id
				projectNetwork2Id := dc.GetNetworkByName("project network 2").Id
				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project1,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeC1Large),
					Image:          "debian-13",
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet},
						{Network: projectNetwork1Id},
						{Network: projectNetwork2Id},
					},
				}
				return req, errorutil.InvalidArgument(`machines must be allocated in exactly one child or child_shared network`)
			},
			want: nil,
		},
		{
			name: "machine with private network in wrong project network",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.FilesystemLayouts = []*adminv2.FilesystemServiceCreateRequest{
					{
						FilesystemLayout: &apiv2.FilesystemLayout{
							Id: "debian",
							Constraints: &apiv2.FilesystemLayoutConstraints{
								Sizes: []string{sc.SizeC1Large},
								Images: map[string]string{
									"debian": ">= 12.0",
								},
							},
						},
					},
				}
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:      new("project network"),
					Project:   new(sc.Tenant1Project1),
					Partition: new(sc.Partition1),
					Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				})
				dc.Create(&testDC)

				projectNetworkId := dc.GetNetworkByName("project network").Id
				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project2,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeC1Large),
					Image:          "debian-13",
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet},
						{Network: projectNetworkId},
					},
				}
				return req, errorutil.InvalidArgument(`given network %s is project scoped but not part of project %s`, projectNetworkId, sc.Tenant1Project2)
			},
			want: nil,
		},
		{
			name: "machine with two child (shared) networks",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.Networks = append(testDC.Networks,
					&adminv2.NetworkServiceCreateRequest{
						Name:      new("project network"),
						Project:   new(sc.Tenant1Project2),
						Partition: new(sc.Partition1),
						Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
					},
					&adminv2.NetworkServiceCreateRequest{
						Name:      new("project shared network 1"),
						Project:   new(sc.Tenant1Project1),
						Partition: new(sc.Partition1),
						Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED,
					},
				)
				dc.Create(&testDC)

				projectNetworkId := dc.GetNetworkByName("project network").Id
				projectSharedNetworkId1 := dc.GetNetworkByName("project shared network 1").Id
				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project2,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeC1Large),
					Image:          "debian-13",
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet},
						{Network: projectNetworkId},
						{Network: projectSharedNetworkId1},
					},
				}
				return req, errorutil.InvalidArgument(`machines must be allocated in exactly one child or child_shared network`)
			},
			want: nil,
		},
		{
			name: "machine with private network with unknown ips",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.FilesystemLayouts = []*adminv2.FilesystemServiceCreateRequest{
					{
						FilesystemLayout: &apiv2.FilesystemLayout{
							Id: "debian",
							Constraints: &apiv2.FilesystemLayoutConstraints{
								Sizes: []string{sc.SizeC1Large},
								Images: map[string]string{
									"debian": ">= 12.0",
								},
							},
						},
					},
				}
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:      new("project network"),
					Project:   new(sc.Tenant1Project1),
					Partition: new(sc.Partition1),
					Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				})
				dc.Create(&testDC)

				projectNetworkId := dc.GetNetworkByName("project network").Id
				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project1,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeC1Large),
					Image:          "debian-13",
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet},
						{Network: projectNetworkId, Ips: []string{"1.2.3.4"}},
					},
				}
				return req, errorutil.NotFound(`no ip with id "1.2.3.4" found`)
			},
			want: nil,
		},
		{
			name: "machine with private network with ips from different network",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.FilesystemLayouts = []*adminv2.FilesystemServiceCreateRequest{
					{
						FilesystemLayout: &apiv2.FilesystemLayout{
							Id: "debian",
							Constraints: &apiv2.FilesystemLayoutConstraints{
								Sizes: []string{sc.SizeC1Large},
								Images: map[string]string{
									"debian": ">= 12.0",
								},
							},
						},
					},
				}
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:      new("project network"),
					Project:   new(sc.Tenant1Project1),
					Partition: new(sc.Partition1),
					Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				})
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:      new("2nd project network"),
					Project:   new(sc.Tenant1Project1),
					Partition: new(sc.Partition1),
					Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				})
				dc.Create(&testDC)

				ipcr, err := dc.GetTestStore().UnscopedIP().Create(t.Context(), &apiv2.IPServiceCreateRequest{
					Network: dc.GetNetworkByName("2nd project network").Id,
					Project: sc.Tenant1Project1,
					Name:    new("ip-project-2"),
				})
				require.NoError(t, err)

				projectNetworkId := dc.GetNetworkByName("project network").Id
				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project1,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeC1Large),
					Image:          "debian-13",
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet},
						{Network: projectNetworkId, Ips: []string{ipcr.Ip}},
					},
				}
				return req, errorutil.InvalidArgument(`given ip %s is not in the given network %s, which is required`, projectNetworkId, ipcr.Ip)
			},
			want: nil,
		},
		{
			name: "machine with private network with ips from different project",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:      new("project network"),
					Project:   new(sc.Tenant1Project1),
					Partition: new(sc.Partition1),
					Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				})
				dc.Create(&testDC)

				ipcr, err := dc.GetTestStore().UnscopedIP().Create(t.Context(), &apiv2.IPServiceCreateRequest{
					Network: sc.NetworkInternet,
					Project: sc.Tenant1Project2,
					Name:    new("ip-project-2"),
				})
				require.NoError(t, err)

				projectNetworkId := dc.GetNetworkByName("project network").Id
				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project1,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeC1Large),
					Image:          sc.ImageDebian13,
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet, Ips: []string{ipcr.Ip}},
						{Network: projectNetworkId},
					},
				}
				return req, errorutil.InvalidArgument(`given ip %s is not in the allocation project`, ipcr.Ip)
			},
			want: nil,
		},
		{
			name: "machine with private network with no ips left",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:      new("project network"),
					Project:   new(sc.Tenant1Project1),
					Partition: new(sc.Partition1),
					Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
					Length:    &apiv2.ChildPrefixLength{Ipv4: new(uint32(30))},
				})
				dc.Create(&testDC)
				projectNetworkId := dc.GetNetworkByName("project network").Id

				_, err := dc.GetTestStore().UnscopedIP().Create(t.Context(), &apiv2.IPServiceCreateRequest{
					Network: projectNetworkId,
					Project: sc.Tenant1Project1,
					Name:    new("ip-1"),
				})
				require.NoError(t, err)
				_, err = dc.GetTestStore().UnscopedIP().Create(t.Context(), &apiv2.IPServiceCreateRequest{
					Network: projectNetworkId,
					Project: sc.Tenant1Project1,
					Name:    new("ip-2"),
				})
				require.NoError(t, err)

				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project1,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeC1Large),
					Image:          sc.ImageDebian13,
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: projectNetworkId},
					},
				}
				return req, errorutil.InvalidArgument(`no free ips in network %s`, projectNetworkId)
			},
			want: nil,
		},
		{
			name: "machine with private network and size-imageconstraints",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:      new("project network"),
					Project:   new(sc.Tenant1Project1),
					Partition: new(sc.Partition1),
					Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				})
				testDC.SizeImageConstraints = append(testDC.SizeImageConstraints, &adminv2.SizeImageConstraintServiceCreateRequest{
					Name: new("c1-large-not-with-debian13"),
					Size: sc.SizeC1Large,
					ImageConstraints: []*apiv2.ImageConstraint{
						{
							Image:       "debian",
							SemverMatch: "<= 12.0",
						},
					},
				})
				dc.Create(&testDC)

				projectNetworkId := dc.GetNetworkByName("project network").Id
				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project1,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeC1Large),
					Image:          sc.ImageDebian13,
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet},
						{Network: projectNetworkId},
					},
				}
				return req, errorutil.InvalidArgument(`given size:c1-large-x86 with image:debian-13.0.20260131 does violate image constraint:debian <=12.0`)
			},
			want: nil,
		},
		{
			name: "machine with private network and reservations",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:      new("project network"),
					Project:   new(sc.Tenant1Project1),
					Partition: new(sc.Partition1),
					Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				})
				testDC.SizeReservations = append(testDC.SizeReservations, &adminv2.SizeReservationServiceCreateRequest{
					SizeReservation: &apiv2.SizeReservation{
						Name:       "reserve 10 c1-large in partition one for tenant1 project2",
						Project:    sc.Tenant1Project2,
						Size:       sc.SizeC1Large,
						Partitions: []string{sc.Partition1},
						Amount:     10,
					},
				})
				dc.Create(&testDC)

				projectNetworkId := dc.GetNetworkByName("project network").Id
				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project1,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeC1Large),
					Image:          sc.ImageDebian13,
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet},
						{Network: projectNetworkId},
					},
				}
				return req, errorutil.Internal("no machine available") // FIXME this error should not be internal, validation must be done in the validation
			},
			want: nil,
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

			ctx := token.ContextWithToken(t.Context(), &testToken)
			if tt.ctxFn != nil {
				ctx = tt.ctxFn()
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
			); diff != "" {
				t.Errorf("machineServiceServer.Create() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_machineServiceServer_ValidateCreateFirewall(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()
	// Add token to be able to get the user from the context
	testToken := apiv2.Token{
		User:      "unit-test-user",
		AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
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
			name: "image type wrong",
			req: &apiv2.MachineServiceCreateRequest{
				Project:        sc.Tenant1Project1,
				Partition:      new(sc.Partition1),
				Size:           new(sc.SizeN1Medium),
				Image:          sc.ImageDebian12,
				AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL,
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`given image %s is not allowed for firewalls`, sc.ImageDebian12),
		},
		{
			name: "firewall spec not valid",
			req: &apiv2.MachineServiceCreateRequest{
				Project:        sc.Tenant1Project1,
				Partition:      new(sc.Partition1),
				Size:           new(sc.SizeN1Medium),
				Image:          sc.ImageFirewall3_0,
				AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL,
				FirewallSpec: &apiv2.FirewallSpec{
					FirewallRules: &apiv2.FirewallRules{
						Egress: []*apiv2.FirewallEgressRule{
							{
								Protocol: apiv2.IPProtocol_IP_PROTOCOL_TCP,
								Ports:    []uint32{80, 443},
								To:       []string{"0.0.0.0.0/0"},
							},
						},
					},
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`egress rule with error:invalid cidr: netip.ParsePrefix("0.0.0.0.0/0"): ParseAddr("0.0.0.0.0"): IPv4 address too long`),
		},
		{
			name: "firewall without external network",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:      new("project network"),
					Project:   new(sc.Tenant1Project1),
					Partition: new(sc.Partition1),
					Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				})
				dc.Create(&testDC)

				projectNetworkId := dc.GetNetworkByName("project network").Id
				req := &apiv2.MachineServiceCreateRequest{
					Name:           "testfirewall",
					Project:        sc.Tenant1Project1,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeN1Medium),
					Image:          sc.ImageFirewall3_0,
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL,
					Networks: []*apiv2.MachineAllocationNetwork{
						// {Network: sc.NetworkInternet},
						{Network: projectNetworkId},
					},
				}
				return req, errorutil.InvalidArgument(`firewalls must be allocated in at least one external network`)
			},
			want: nil,
		},
		{
			name: "firewall with underlay specified",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.Networks = append(testDC.Networks, &adminv2.NetworkServiceCreateRequest{
					Name:      new("project network"),
					Project:   new(sc.Tenant1Project1),
					Partition: new(sc.Partition1),
					Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
				})
				dc.Create(&testDC)

				projectNetworkId := dc.GetNetworkByName("project network").Id
				req := &apiv2.MachineServiceCreateRequest{
					Name:           "testfirewall",
					Project:        sc.Tenant1Project1,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeN1Medium),
					Image:          sc.ImageFirewall3_0,
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkUnderlayPartition1},
						{Network: projectNetworkId},
					},
				}
				return req, errorutil.InvalidArgument(`underlays cannot be specified in a machine allocation request (this is done automatically for firewalls)`)
			},
			want: nil,
		},
		{
			name: "firewall but underlay does not have ips available anymore",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.Partitions = append(testDC.Partitions, "partition-2")
				testDC.Networks = append(testDC.Networks,
					&adminv2.NetworkServiceCreateRequest{
						Id:                       new("tenant-super-partition-2"),
						Partition:                new("partition-2"),
						Prefixes:                 []string{"19.110.0.0/16"},
						DestinationPrefixes:      []string{"1.2.3.0/24"},
						DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: new(uint32(22))},
						Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
					},
					&adminv2.NetworkServiceCreateRequest{
						Id:        new("underlay-partition-2"),
						Prefixes:  []string{"10.251.0.0/31"},
						Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY,
						Partition: new("partition-2"),
					},
					&adminv2.NetworkServiceCreateRequest{
						Name:      new("project network"),
						Project:   new(sc.Tenant1Project1),
						Partition: new("partition-2"),
						Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
					},
				)
				dc.Create(&testDC)

				projectNetworkId := dc.GetNetworkByName("project network").Id
				req := &apiv2.MachineServiceCreateRequest{
					Name:           "testfirewall",
					Project:        sc.Tenant1Project1,
					Partition:      new("partition-2"),
					Size:           new(sc.SizeN1Medium),
					Image:          sc.ImageFirewall3_0,
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet},
						{Network: projectNetworkId},
					},
				}
				return req, errorutil.InvalidArgument(`no free ips in network underlay-partition-2`)
			},
			want: nil,
		},
		{
			name: "firewall with no child shared networks",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				dc.Create(&testDC)

				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project2,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeN1Medium),
					Image:          sc.ImageFirewall3_0,
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet},
					},
				}
				return req, errorutil.InvalidArgument(`firewalls must have at least one child or child_shared network`)
			},
			want: nil,
		},
		{
			name: "firewall with two child shared networks",
			req:  nil, // set below
			createRequestFn: func() (*apiv2.MachineServiceCreateRequest, error) {
				testDC := sc.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.Networks = append(testDC.Networks,
					&adminv2.NetworkServiceCreateRequest{
						Name:      new("project network"),
						Project:   new(sc.Tenant1Project2),
						Partition: new(sc.Partition1),
						Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD,
					},
					&adminv2.NetworkServiceCreateRequest{
						Name:      new("project shared network 1"),
						Project:   new(sc.Tenant1Project1),
						Partition: new(sc.Partition1),
						Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED,
					},
					&adminv2.NetworkServiceCreateRequest{
						Name:      new("project shared network 2"),
						Project:   new(sc.Tenant1Project2),
						Partition: new(sc.Partition1),
						Type:      apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED,
					},
				)
				dc.Create(&testDC)

				projectNetworkId := dc.GetNetworkByName("project network").Id
				projectSharedNetworkId1 := dc.GetNetworkByName("project shared network 1").Id
				projectSharedNetworkId2 := dc.GetNetworkByName("project shared network 2").Id
				req := &apiv2.MachineServiceCreateRequest{
					Project:        sc.Tenant1Project2,
					Partition:      new(sc.Partition1),
					Size:           new(sc.SizeN1Medium),
					Image:          sc.ImageFirewall3_0,
					AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL,
					Networks: []*apiv2.MachineAllocationNetwork{
						{Network: sc.NetworkInternet},
						{Network: projectNetworkId},
						{Network: projectSharedNetworkId1},
						{Network: projectSharedNetworkId2},
					},
				}
				return req, errorutil.InvalidArgument(`machines or firewalls must not be allocated in more than one child_shared network`)
			},
			want: nil,
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
			); diff != "" {
				t.Errorf("machineServiceServer.Create() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}
