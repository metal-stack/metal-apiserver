package test

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAssert(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := NewDatacenter(t, log)
	defer dc.Close()

	tests := []struct {
		name    string
		spec    *sc.DatacenterSpec
		mods    func() *Asserters
		wantErr bool
	}{
		{
			name:    "no modification, both equal",
			spec:    &sc.DefaultDatacenter,
			wantErr: false,
		},
		{
			name: "no modification, but datacenters differ",
			spec: &sc.DefaultDatacenter,
			mods: func() *Asserters {
				_, err := dc.GetTestStore().Partition().Update(ctx, sc.Partition1, &adminv2.PartitionServiceUpdateRequest{
					Id:          sc.Partition1,
					Description: new("changed"),
					UpdateMeta: &apiv2.UpdateMeta{
						LockingStrategy: apiv2.OptimisticLockingStrategy_OPTIMISTIC_LOCKING_STRATEGY_SERVER,
					},
				})
				require.NoError(t, err)

				return nil
			},
			wantErr: true,
		},
		{
			name: "apply correct modification",
			spec: &sc.DefaultDatacenter,
			mods: func() *Asserters {
				_, err := dc.GetTestStore().Partition().Update(ctx, sc.Partition1, &adminv2.PartitionServiceUpdateRequest{
					Id:          sc.Partition1,
					Description: new("changed"),
					UpdateMeta: &apiv2.UpdateMeta{
						LockingStrategy: apiv2.OptimisticLockingStrategy_OPTIMISTIC_LOCKING_STRATEGY_SERVER,
					},
				})
				require.NoError(t, err)

				return &Asserters{
					Partitions: func(partitions map[string]*apiv2.Partition) {
						partitions[sc.Partition1].Description = "changed"
					},
					Machines: func(machines map[string]*apiv2.Machine) {
						machines[sc.Machine1].Partition.Description = "changed"
					},
				}
			},
			wantErr: false,
		},
		{
			name: "apply wrong modification",
			spec: &sc.DefaultDatacenter,
			mods: func() *Asserters {
				_, err := dc.GetTestStore().Partition().Update(ctx, sc.Partition1, &adminv2.PartitionServiceUpdateRequest{
					Id:          sc.Partition1,
					Description: new("changed"),
					UpdateMeta: &apiv2.UpdateMeta{
						LockingStrategy: apiv2.OptimisticLockingStrategy_OPTIMISTIC_LOCKING_STRATEGY_SERVER,
					},
				})
				require.NoError(t, err)

				return &Asserters{
					Sizes: func(sizes map[string]*apiv2.Size) {
						sizes[sc.SizeC1Large].Description = new("falsely changed")
					},
				}
			},
			wantErr: true,
		},
		{
			name: "new entity added, but no modify passed",
			spec: &sc.DefaultDatacenter,
			mods: func() *Asserters {
				_, err := dc.GetTestStore().Partition().Create(ctx, &adminv2.PartitionServiceCreateRequest{
					Partition: &apiv2.Partition{
						Id: "partition-2",
					},
				})
				require.NoError(t, err)

				return nil
			},
			wantErr: true,
		},
		{
			name: "new entities added and correct modifications applied",
			spec: &sc.DefaultDatacenter,
			mods: func() *Asserters {
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = fmt.Fprintln(w, "a image")
				}))
				defer ts.Close()

				p, err := dc.GetTestStore().UnscopedProject().AdditionalMethods().CreateWithID(ctx, &apiv2.ProjectServiceCreateRequest{
					Login: "john.doe",
					Name:  "project-2",
				}, "20000000-0000-0000-0000-000000000001")
				require.NoError(t, err)

				_, err = dc.GetTestStore().Partition().Create(ctx, &adminv2.PartitionServiceCreateRequest{
					Partition: &apiv2.Partition{
						Id: "partition-2",
					},
				})
				require.NoError(t, err)

				sz, err := dc.GetTestStore().Size().Create(ctx, &adminv2.SizeServiceCreateRequest{
					Size: &apiv2.Size{
						Id: "size-2",
						Constraints: []*apiv2.SizeConstraint{
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 3, Max: 3},
						},
					},
				})
				require.NoError(t, err)

				nw, err := dc.GetTestStore().UnscopedNetwork().Create(ctx, &adminv2.NetworkServiceCreateRequest{
					Id:       new("network-2"),
					Type:     apiv2.NetworkType_NETWORK_TYPE_EXTERNAL,
					Project:  new("20000000-0000-0000-0000-000000000001"),
					Prefixes: []string{"1.1.1.0/24"},
					Vrf:      new(uint32(43)),
				})
				require.NoError(t, err)

				ip, err := dc.GetTestStore().UnscopedIP().Create(ctx, &apiv2.IPServiceCreateRequest{
					Network: "network-2",
					Project: "20000000-0000-0000-0000-000000000001",
					Ip:      new("1.1.1.1"),
				})
				require.NoError(t, err)

				image, err := dc.GetTestStore().Image().Create(ctx, &adminv2.ImageServiceCreateRequest{
					Image: &apiv2.Image{
						Id:       "debian-11.0",
						Url:      ts.URL,
						Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
					},
				})
				require.NoError(t, err)

				sw, err := dc.GetTestStore().Switch().Create(ctx, &api.SwitchServiceCreateRequest{
					Switch: &apiv2.Switch{
						Id:        "sw3",
						Partition: "partition-2",
						Os: &apiv2.SwitchOS{
							Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
						},
					},
				})
				require.NoError(t, err)
				sync := &apiv2.SwitchSync{
					Duration: &durationpb.Duration{},
					Time:     &timestamppb.Timestamp{},
				}
				sw.LastSync = sync
				sw.LastSyncError = sync

				err = dc.GetTestStore().Switch().AdditionalMethods().SetSwitchStatus(ctx, &api.SwitchStatus{
					ID:            sw.Id,
					LastSync:      sync,
					LastSyncError: sync,
				})
				require.NoError(t, err)

				return &Asserters{
					Projects: func(projects map[string][]*apiv2.Project) {
						projects["john.doe"] = append(projects["john.doe"], &apiv2.Project{
							Uuid:   p.Meta.Id,
							Meta:   &apiv2.Meta{},
							Tenant: p.TenantId,
							Name:   p.Name,
						})
					},
					Partitions: func(partitions map[string]*apiv2.Partition) {
						partitions["partition-2"] = &apiv2.Partition{
							Id:                "partition-2",
							Meta:              &apiv2.Meta{},
							BootConfiguration: &apiv2.PartitionBootConfiguration{},
						}
					},
					Sizes: func(sizes map[string]*apiv2.Size) {
						sizes[sz.Id] = sz
					},
					Networks: func(networks map[string]*apiv2.Network) {
						networks[nw.Id] = nw
						networks["network-2"].Consumption.Ipv4.UsedIps = 3
					},
					IPs: func(ips map[string]*apiv2.IP) {
						ips[ip.Ip] = ip
					},
					Images: func(images map[string]*apiv2.Image) {
						images[image.Id] = image
					},
					Switches: func(switches map[string]*apiv2.Switch) {
						switches[sw.Id] = sw
					},
					SwitchStatuses: func(switchStatuses map[string]*metal.SwitchStatus) {
						switchStatuses[sw.Id] = &metal.SwitchStatus{
							Base: metal.Base{
								ID: sw.Id,
							},
							LastSync: &metal.SwitchSync{
								Time: time.Unix(0, 0),
							},
							LastSyncError: &metal.SwitchSync{
								Time: time.Unix(0, 0),
							},
						}
					},
				}
			},
			wantErr: false,
		},
		{
			name: "entity deleted, but no modification applied",
			spec: &sc.DefaultDatacenter,
			mods: func() *Asserters {
				_, err := dc.GetTestStore().Switch().AdditionalMethods().ForceDelete(ctx, sc.P01Rack01Switch1)
				require.NoError(t, err)
				return nil
			},
			wantErr: true,
		},
		{
			name: "entity deleted and correct modifications applied",
			spec: &sc.DefaultDatacenter,
			mods: func() *Asserters {
				_, err := dc.GetTestStore().Switch().AdditionalMethods().ForceDelete(ctx, sc.P01Rack01Switch1)
				require.NoError(t, err)

				return &Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						delete(switches, sc.P01Rack01Switch1)
					},
					SwitchStatuses: func(switchStatuses map[string]*metal.SwitchStatus) {
						delete(switchStatuses, sc.P01Rack01Switch1)
					},
				}
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.Create(tt.spec)
			defer dc.Cleanup()

			snapshot := dc.Snapshot()

			var mods *Asserters
			if tt.mods != nil {
				mods = tt.mods()
			}

			err1 := dc.AssertSnapshot(snapshot, mods,
				protocmp.IgnoreFields(
					&apiv2.IP{}, "uuid",
				),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "classification", "description", "expires_at", "name", "url",
				),
			)
			if (err1 != nil) != tt.wantErr {
				t.Errorf("Assert() error = %v, wantErr %v", err1, tt.wantErr)
			}

			err2 := dc.AssertSnapshot(snapshot, mods,
				protocmp.IgnoreFields(
					&apiv2.IP{}, "uuid",
				),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "classification", "description", "expires_at", "name", "url",
				),
			)
			if diff := cmp.Diff(err1, err2, errorutil.ErrorStringComparer()); diff != "" {
				t.Errorf("Assert() is not idempotent; err1 = %s, err2 = %s", err1, err2)
			}
		})
	}
}

func Test_entities_deepCopy(t *testing.T) {
	tests := []struct {
		name string
		e    *Entities
		want *Entities
	}{
		{
			name: "copy all entities",
			e: &Entities{
				Tenants: map[string]*apiv2.Tenant{
					"tenant": {
						Login: "tenant",
					},
				},
				Projects: map[string][]*apiv2.Project{
					"tenant": {
						{
							Name: "project",
						},
					},
				},
				Partitions: map[string]*apiv2.Partition{
					"partition": {
						Id: "partition",
					},
				},
				Sizes: map[string]*apiv2.Size{
					"size": {
						Id: "size",
					},
				},
				Networks: map[string]*apiv2.Network{
					"network": {
						Id: "network",
					},
				},
				Ips: map[string]*apiv2.IP{
					"ip": {
						Ip: "1.1.1.1",
					},
				},
				Images: map[string]*apiv2.Image{
					"image": {
						Id: "image",
					},
				},
				Switches: map[string]*apiv2.Switch{
					"switch": {
						Id: "switch",
					},
				},
				SwitchStatuses: map[string]*metal.SwitchStatus{
					"switch": {
						Base: metal.Base{
							ID: "switch",
						},
					},
				},
				Machines: map[string]*apiv2.Machine{
					"machine": {
						Uuid: "machine",
					},
				},
			},
			want: &Entities{
				Tenants: map[string]*apiv2.Tenant{
					"tenant": {
						Login: "tenant",
					},
				},
				Projects: map[string][]*apiv2.Project{
					"tenant": {
						{
							Name: "project",
						},
					},
				},
				Partitions: map[string]*apiv2.Partition{
					"partition": {
						Id: "partition",
					},
				},
				Sizes: map[string]*apiv2.Size{
					"size": {
						Id: "size",
					},
				},
				Networks: map[string]*apiv2.Network{
					"network": {
						Id: "network",
					},
				},
				Ips: map[string]*apiv2.IP{
					"ip": {
						Ip: "1.1.1.1",
					},
				},
				Images: map[string]*apiv2.Image{
					"image": {
						Id: "image",
					},
				},
				Switches: map[string]*apiv2.Switch{
					"switch": {
						Id: "switch",
					},
				},
				SwitchStatuses: map[string]*metal.SwitchStatus{
					"switch": {
						Base: metal.Base{
							ID: "switch",
						},
					},
				},
				Machines: map[string]*apiv2.Machine{
					"machine": {
						Uuid: "machine",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.e.deepCopy()
			require.NoError(t, err)
			if diff := cmp.Diff(tt.want, got, cmp.AllowUnexported(Entities{}), protocmp.Transform()); diff != "" {
				t.Errorf("entities.deepCopy() diff = %s", diff)
			}
		})
	}
}

func Test_deepCopy(t *testing.T) {
	type testStruct struct {
		unexported  string
		StringValue string
		StructValue *testStruct
	}
	tests := []struct {
		name string
		in   testStruct
		want testStruct
	}{
		{
			name: "copy all exported fields by value",
			in: testStruct{
				unexported:  "unexported",
				StringValue: "some value",
				StructValue: &testStruct{
					StringValue: "some other value",
					StructValue: &testStruct{},
				},
			},
			want: testStruct{
				unexported:  "",
				StringValue: "some value",
				StructValue: &testStruct{
					StringValue: "some other value",
					StructValue: &testStruct{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := deepCopy(tt.in)
			require.NoError(t, err)
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreUnexported(testStruct{})); diff != "" {
				t.Errorf("deepCopy() diff = %s", diff)
			}
			in := tt.in
			if got == in {
				t.Errorf("deepCopy() copied struct by reference")
			}
			if got.unexported == in.unexported {
				t.Errorf("deepCopy() copied unexported value")
			}
			if got.StructValue == in.StructValue {
				t.Errorf("deepCopy() copied struct value by reference")
			}
		})
	}
}
