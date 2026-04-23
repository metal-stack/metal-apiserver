package test_test

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
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestAssert(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	tests := []struct {
		name    string
		spec    *sc.DatacenterSpec
		mods    func() *test.Asserters
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
			mods: func() *test.Asserters {
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
			mods: func() *test.Asserters {
				_, err := dc.GetTestStore().Partition().Update(ctx, sc.Partition1, &adminv2.PartitionServiceUpdateRequest{
					Id:          sc.Partition1,
					Description: new("changed"),
					UpdateMeta: &apiv2.UpdateMeta{
						LockingStrategy: apiv2.OptimisticLockingStrategy_OPTIMISTIC_LOCKING_STRATEGY_SERVER,
					},
				})
				require.NoError(t, err)

				return &test.Asserters{
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
			mods: func() *test.Asserters {
				_, err := dc.GetTestStore().Partition().Update(ctx, sc.Partition1, &adminv2.PartitionServiceUpdateRequest{
					Id:          sc.Partition1,
					Description: new("changed"),
					UpdateMeta: &apiv2.UpdateMeta{
						LockingStrategy: apiv2.OptimisticLockingStrategy_OPTIMISTIC_LOCKING_STRATEGY_SERVER,
					},
				})
				require.NoError(t, err)

				return &test.Asserters{
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
			mods: func() *test.Asserters {
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
			mods: func() *test.Asserters {
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

				err = dc.GetTestStore().Switch().AdditionalMethods().SetSwitchStatus(ctx, &api.SwitchStatus{
					ID: sw.Id,
				})
				require.NoError(t, err)

				return &test.Asserters{
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
						}
					},
				}
			},
			wantErr: false,
		},
		{
			name: "entity deleted, but no modification applied",
			spec: &sc.DefaultDatacenter,
			mods: func() *test.Asserters {
				_, err := dc.GetTestStore().Switch().AdditionalMethods().ForceDelete(ctx, sc.SwmP01Rack01Switch1)
				require.NoError(t, err)
				return nil
			},
			wantErr: true,
		},
		{
			name: "entity deleted and correct modifications applied",
			spec: &sc.DefaultDatacenter,
			mods: func() *test.Asserters {
				_, err := dc.GetTestStore().Switch().AdditionalMethods().ForceDelete(ctx, sc.SwmP01Rack01Switch1)
				require.NoError(t, err)

				return &test.Asserters{
					Switches: func(switches map[string]*apiv2.Switch) {
						delete(switches, sc.SwmP01Rack01Switch1)
					},
					SwitchStatuses: func(switchStatuses map[string]*metal.SwitchStatus) {
						delete(switchStatuses, sc.SwmP01Rack01Switch1)
					},
				}
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer dc.Cleanup()

			dc.Create(tt.spec)

			var mods *test.Asserters
			if tt.mods != nil {
				mods = tt.mods()
			}

			err1 := dc.Assert(mods,
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

			err2 := dc.Assert(mods,
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
