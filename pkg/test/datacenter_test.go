package test_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
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
		before  func()
		modify  func(*test.Datacenter)
		wantErr bool
	}{
		{
			name:    "no modification, both equal",
			spec:    &sc.DefaultDatacenter,
			before:  func() {},
			modify:  nil,
			wantErr: false,
		},
		{
			name: "no modification, but datacenters differ",
			spec: &sc.DefaultDatacenter,
			before: func() {
				_, err := dc.TestStore.Partition().Update(ctx, sc.Partition1, &adminv2.PartitionServiceUpdateRequest{
					Id:          sc.Partition1,
					Description: new("changed"),
					UpdateMeta: &apiv2.UpdateMeta{
						UpdatedAt:       dc.Partitions[sc.Partition1].Meta.CreatedAt,
						LockingStrategy: apiv2.OptimisticLockingStrategy_OPTIMISTIC_LOCKING_STRATEGY_SERVER,
					},
				})
				require.NoError(t, err)
			},
			modify:  nil,
			wantErr: true,
		},
		{
			name: "apply correct modification",
			spec: &sc.DefaultDatacenter,
			before: func() {
				_, err := dc.TestStore.Partition().Update(ctx, sc.Partition1, &adminv2.PartitionServiceUpdateRequest{
					Id:          sc.Partition1,
					Description: new("changed"),
					UpdateMeta: &apiv2.UpdateMeta{
						UpdatedAt:       dc.Partitions[sc.Partition1].Meta.CreatedAt,
						LockingStrategy: apiv2.OptimisticLockingStrategy_OPTIMISTIC_LOCKING_STRATEGY_SERVER,
					},
				})
				require.NoError(t, err)
			},
			modify: func(d *test.Datacenter) {
				d.Partitions[sc.Partition1].Description = "changed"
				d.Machines[sc.Machine1].Partition.Description = "changed"
			},
			wantErr: false,
		},
		{
			name: "apply wrong modification",
			spec: &sc.DefaultDatacenter,
			before: func() {
				_, err := dc.TestStore.Partition().Update(ctx, sc.Partition1, &adminv2.PartitionServiceUpdateRequest{
					Id:          sc.Partition1,
					Description: new("changed"),
					UpdateMeta: &apiv2.UpdateMeta{
						UpdatedAt:       dc.Partitions[sc.Partition1].Meta.CreatedAt,
						LockingStrategy: apiv2.OptimisticLockingStrategy_OPTIMISTIC_LOCKING_STRATEGY_SERVER,
					},
				})
				require.NoError(t, err)
			},
			modify: func(d *test.Datacenter) {
				d.Sizes[sc.SizeC1Large].Description = new("falsely changed")
			},
			wantErr: true,
		},
		{
			name: "new entity added, but no modify passed",
			spec: &sc.DefaultDatacenter,
			before: func() {
				_, err := dc.TestStore.Partition().Create(ctx, &adminv2.PartitionServiceCreateRequest{
					Partition: &apiv2.Partition{
						Id: "partition-2",
					},
				})
				require.NoError(t, err)
			},
			modify:  nil,
			wantErr: true,
		},
		{
			name: "new entities added and correct modifications applied",
			spec: &sc.DefaultDatacenter,
			before: func() {
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = fmt.Fprintln(w, "a image")
				}))
				defer ts.Close()

				_, err := dc.TestStore.UnscopedProject().AdditionalMethods().CreateWithID(ctx, &apiv2.ProjectServiceCreateRequest{
					Login: "john.doe",
					Name:  "project-2",
				}, "20000000-0000-0000-0000-000000000001")
				require.NoError(t, err)

				_, err = dc.TestStore.Partition().Create(ctx, &adminv2.PartitionServiceCreateRequest{
					Partition: &apiv2.Partition{
						Id: "partition-2",
					},
				})
				require.NoError(t, err)

				_, err = dc.TestStore.Size().Create(ctx, &adminv2.SizeServiceCreateRequest{
					Size: &apiv2.Size{
						Id: "size-2",
						Constraints: []*apiv2.SizeConstraint{
							{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 3, Max: 3},
						},
					},
				})
				require.NoError(t, err)

				_, err = dc.TestStore.UnscopedNetwork().Create(ctx, &adminv2.NetworkServiceCreateRequest{
					Id:      new("network-2"),
					Type:    apiv2.NetworkType_NETWORK_TYPE_SUPER,
					Project: new("20000000-0000-0000-0000-000000000001"),
					DefaultChildPrefixLength: &apiv2.ChildPrefixLength{
						Ipv4: new(uint32(32)),
					},
					Prefixes: []string{"1.1.1.0/24"},
				})
				require.NoError(t, err)

				_, err = dc.TestStore.UnscopedIP().Create(ctx, &apiv2.IPServiceCreateRequest{
					Network: "network-2",
					Project: "20000000-0000-0000-0000-000000000001",
					Ip:      new("1.1.1.1"),
				})
				require.NoError(t, err)

				_, err = dc.TestStore.Image().Create(ctx, &adminv2.ImageServiceCreateRequest{
					Image: &apiv2.Image{
						Id:       "debian-11.0",
						Url:      ts.URL,
						Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
					},
				})
				require.NoError(t, err)

				_, err = dc.TestStore.Switch().Create(ctx, &repository.SwitchServiceCreateRequest{
					Switch: &apiv2.Switch{
						Id:        "sw3",
						Partition: "partition-2",
						Os: &apiv2.SwitchOS{
							Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
						},
					},
				})
				require.NoError(t, err)
			},
			modify: func(d *test.Datacenter) {
				d.Projects["john.doe"] = append(d.Projects["john.doe"], "20000000-0000-0000-0000-000000000001")
				d.Partitions["partition-2"] = &apiv2.Partition{
					Id:                "partition-2",
					BootConfiguration: &apiv2.PartitionBootConfiguration{},
				}
				d.Sizes["size-2"] = &apiv2.Size{
					Id: "size-2",
					Constraints: []*apiv2.SizeConstraint{
						{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 3, Max: 3},
					},
				}
				d.Networks["network-2"] = &apiv2.Network{
					Id:      "network-2",
					Type:    new(apiv2.NetworkType_NETWORK_TYPE_SUPER),
					Project: new("20000000-0000-0000-0000-000000000001"),
					DefaultChildPrefixLength: &apiv2.ChildPrefixLength{
						Ipv4: new(uint32(32)),
					},
					Prefixes: []string{"1.1.1.0/24"},
					Consumption: &apiv2.NetworkConsumption{
						Ipv4: &apiv2.NetworkUsage{
							AvailableIps:      256,
							UsedIps:           3,
							AvailablePrefixes: 1,
						},
					},
				}
				d.IPs["1.1.1.1"] = &apiv2.IP{
					Network: "network-2",
					Project: "20000000-0000-0000-0000-000000000001",
					Ip:      "1.1.1.1",
					Type:    apiv2.IPType_IP_TYPE_EPHEMERAL,
				}
				d.Images["debian-11.0"] = &apiv2.Image{
					Id:       "debian-11.0",
					Features: []apiv2.ImageFeature{apiv2.ImageFeature_IMAGE_FEATURE_MACHINE},
				}
				d.Switches["sw3"] = &apiv2.Switch{
					Id:        "sw3",
					Partition: "partition-2",
					Os: &apiv2.SwitchOS{
						Vendor: apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
					},
				}
			},
			wantErr: false,
		},
		{
			name: "entity deleted, but no modification applied",
			spec: &sc.DefaultDatacenter,
			before: func() {
				_, err := dc.TestStore.Switch().AdditionalMethods().ForceDelete(ctx, "sw1-partition-1-rack-1")
				require.NoError(t, err)
			},
			modify:  nil,
			wantErr: true,
		},
		{
			name: "entities deleted and correct modifications applied",
			spec: &sc.DefaultDatacenter,
			before: func() {
				_, err := dc.TestStore.Switch().AdditionalMethods().ForceDelete(ctx, "sw1-partition-1-rack-1")
				require.NoError(t, err)
			},
			modify: func(d *test.Datacenter) {
				delete(dc.Switches, "sw1-partition-1-rack-1")
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.CleanUp()
			dc.Create(tt.spec)
			tt.before()

			if err := dc.Assert(tt.modify,
				protocmp.IgnoreFields(
					&apiv2.IP{}, "uuid",
				),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "classification", "description", "expires_at", "name", "url",
				),
			); (err != nil) != tt.wantErr {
				t.Errorf("Assert() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
