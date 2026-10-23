package q_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	_ "github.com/lib/pq"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/tag"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg/q"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

var (
	pgIPProjectID = "project-1"
	pgIPNamespace = "namespace-1"

	pgIP1 = &metal.IP{
		IPAddress:        "10.0.0.1",
		AllocationUUID:   "uuid-1",
		Namespace:        nil,
		ParentPrefixCidr: "10.0.0.0/8",
		Name:             "ip-one",
		Description:      "IP 1",
		ProjectID:        pgIPProjectID,
		NetworkID:        "n1",
		Type:             metal.Ephemeral,
		Tags:             []string{"color=red", "size=small", fmt.Sprintf("%s=m1", tag.MachineID)},
	}
	pgIP2 = &metal.IP{
		IPAddress:        "10.0.0.2",
		AllocationUUID:   "uuid-2",
		Namespace:        nil,
		ParentPrefixCidr: "10.0.0.0/8",
		Name:             "ip-two",
		Description:      "IP 2",
		ProjectID:        pgIPProjectID,
		NetworkID:        "n2",
		Type:             metal.Static,
		Tags:             []string{"color=green", "size=medium"},
	}
	pgIP3 = &metal.IP{
		IPAddress:        "10.0.0.3",
		AllocationUUID:   "uuid-3",
		Namespace:        &pgIPNamespace,
		ParentPrefixCidr: "10.0.1.0/24",
		Name:             "ip-three",
		Description:      "IP 3",
		ProjectID:        pgIPProjectID,
		NetworkID:        "n3",
		Type:             metal.Ephemeral,
		Tags:             []string{"color=blue"},
	}
	pgIP4 = &metal.IP{
		IPAddress:        "10.0.0.4",
		AllocationUUID:   "uuid-4",
		Namespace:        nil,
		ParentPrefixCidr: "10.0.0.0/8",
		Name:             "ip-four",
		Description:      "IP 4",
		ProjectID:        "project-2",
		NetworkID:        "n1",
		Type:             metal.Static,
		Tags:             []string{"color=yellow"},
	}
	pgIP5 = &metal.IP{
		IPAddress:        "fd12:34:56:78::1",
		AllocationUUID:   "uuid-5",
		Namespace:        nil,
		ParentPrefixCidr: "fd12:34:56:78::/64",
		Name:             "ip-v6",
		Description:      "IPv6 IP",
		ProjectID:        pgIPProjectID,
		NetworkID:        "n4",
		Type:             metal.Ephemeral,
		Tags:             []string{},
	}
	pgIPs = []*metal.IP{pgIP1, pgIP2, pgIP3, pgIP4, pgIP5}

	pgSize1 = &metal.Size{
		ID: "s1", Name: "s1", Description: "Size 1",
		Labels:      map[string]string{"tier": "standard", "region": "eu"},
		Constraints: []metal.Constraint{{Type: metal.CoreConstraint, Min: 4, Max: 4}},
	}
	pgSize2 = &metal.Size{
		ID: "s2", Name: "s2", Description: "Size 2",
		Labels:      map[string]string{"tier": "premium", "region": "eu"},
		Constraints: []metal.Constraint{{Type: metal.CoreConstraint, Min: 8, Max: 16}},
	}
	pgSize3 = &metal.Size{
		ID: "s3", Name: "s3", Description: "Size 3",
		Labels:      map[string]string{"tier": "standard", "region": "us"},
		Constraints: []metal.Constraint{{Type: metal.CoreConstraint, Min: 4, Max: 8}, {Type: metal.MemoryConstraint, Min: 8192, Max: 16384}},
	}
	pgSize4 = &metal.Size{
		ID: "gpu-size", Name: "gpu-size", Description: "GPU Size",
		Labels:      map[string]string{"tier": "premium", "region": "eu", "gpu": "nvidia"},
		Constraints: []metal.Constraint{{Type: metal.GPUConstraint, Min: 1, Max: 4, Identifier: "a100"}},
	}
	pgSizes = []*metal.Size{pgSize1, pgSize2, pgSize3, pgSize4}

	pgSizeReservation1 = &metal.SizeReservation{
		ID: "res1", Name: "res1", Description: "Res 1",
		SizeID:       "s1",
		Amount:       1,
		ProjectID:    "p1",
		PartitionIDs: []string{"partition-a"},
		Labels:       map[string]string{"tier": "standard"},
	}
	pgSizeReservation2 = &metal.SizeReservation{
		ID: "res2", Name: "res2", Description: "Res 2",
		SizeID:       "s2",
		Amount:       3,
		ProjectID:    "p2",
		PartitionIDs: []string{"partition-b", "partition-c"},
		Labels:       map[string]string{"tier": "premium"},
	}
	pgSizeReservations = []*metal.SizeReservation{pgSizeReservation1, pgSizeReservation2}

	pgSizeImageConstraint1 = &metal.SizeImageConstraint{
		ID: "size-1", Name: "size-1", Description: "Size 1 Constraint",
		Images: map[string]string{"debian": ">= 11.0.0", "ubuntu": ">= 22.04.0"},
	}
	pgSizeImageConstraint2 = &metal.SizeImageConstraint{
		ID: "size-2", Name: "size-2", Description: "Size 2 Constraint",
		Images: map[string]string{"debian": ">= 12.0.0", "fedora": ">= 38.0"},
	}
	pgSizeImageConstraint3 = &metal.SizeImageConstraint{
		ID: "size-3", Name: "size-3", Description: "Size 3 Constraint",
		Images: map[string]string{"ubuntu": ">= 20.04.0"},
	}
	pgSizeImageConstraint4 = &metal.SizeImageConstraint{
		ID: "firewall-size", Name: "firewall-size", Description: "Firewall Size Constraint",
		Images: map[string]string{"metalstack": ">= 1.0.0"},
	}
	pgSizeImageConstraints = []*metal.SizeImageConstraint{pgSizeImageConstraint1, pgSizeImageConstraint2, pgSizeImageConstraint3, pgSizeImageConstraint4}

	pgImage1 = &metal.Image{
		ID: "debian-11", Name: "debian-11", Description: "Debian 11",
		OS:             "debian",
		Version:        "11",
		URL:            "https://example.com/debian-11.tgz",
		Classification: metal.ClassificationSupported,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true},
	}
	pgImage2 = &metal.Image{
		ID: "debian-12", Name: "debian-12", Description: "Debian 12",
		OS:             "debian",
		Version:        "12",
		URL:            "https://example.com/debian-12.tgz",
		Classification: metal.ClassificationSupported,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true},
	}
	pgImage3 = &metal.Image{
		ID: "ubuntu-22", Name: "ubuntu-22", Description: "Ubuntu 22",
		OS:             "ubuntu",
		Version:        "22",
		URL:            "https://example.com/ubuntu-22.tgz",
		Classification: metal.ClassificationSupported,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true, metal.ImageFeatureFirewall: true},
		Labels:         map[string]string{"a": "b"},
	}
	pgImage4 = &metal.Image{
		ID: "debian-10", Name: "debian-10", Description: "Old Debian",
		OS:             "debian",
		Version:        "10",
		URL:            "https://example.com/debian-10.tgz",
		Classification: metal.ClassificationDeprecated,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true},
	}
	pgImage5 = &metal.Image{
		ID: "firewall-image", Name: "firewall-only", Description: "Firewall Image",
		OS:             "metalstack",
		Version:        "1",
		URL:            "https://example.com/firewall.tgz",
		Classification: metal.ClassificationSupported,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureFirewall: true},
	}
	pgImage6 = &metal.Image{
		ID: "preview-image", Name: "preview-os", Description: "Preview Image",
		OS:             "previewos",
		Version:        "1",
		URL:            "https://example.com/preview.tgz",
		Classification: metal.ClassificationPreview,
		Features:       map[metal.ImageFeatureType]bool{metal.ImageFeatureMachine: true},
	}
	pgImages = []*metal.Image{pgImage1, pgImage2, pgImage3, pgImage4, pgImage5, pgImage6}

	pgPartition1 = &metal.Partition{ID: "partition-a", Name: "partition-a", Description: "Partition A"}
	pgPartition2 = &metal.Partition{ID: "partition-b", Name: "partition-b", Description: "Partition B"}
	pgPartitions = []*metal.Partition{pgPartition1, pgPartition2}

	pgSwitch1 = &metal.Switch{
		ID:        "sw1",
		Rack:      "rack01",
		Room:      "room01",
		Partition: "partition-a",
		OS:        &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus, Version: "5.9"},
		Nics:      metal.Nics{},
		MachineConnections: metal.ConnectionMap{
			"m1": []metal.Connection{{Nic: metal.Nic{Neighbors: metal.Nics{}}, MachineID: "m1"}},
		},
	}
	pgSwitch2 = &metal.Switch{
		ID:        "sw2",
		Rack:      "rack01",
		Partition: "partition-b",
		OS:        &metal.SwitchOS{Vendor: metal.SwitchOSVendorCumulus, Version: "5.6"},
		Nics:      metal.Nics{},
		MachineConnections: metal.ConnectionMap{
			"m1": []metal.Connection{{Nic: metal.Nic{Neighbors: metal.Nics{}}, MachineID: "m1"}},
		},
	}
	pgSwitch3 = &metal.Switch{
		ID:        "sw3",
		Rack:      "rack02",
		Partition: "partition-a",
		OS:        &metal.SwitchOS{Vendor: metal.SwitchOSVendorSonic, Version: "ec202111.11"},
		Nics:      metal.Nics{},
	}
	pgSwitches = []*metal.Switch{pgSwitch1, pgSwitch2, pgSwitch3}

	pgFilesystemLayout1 = &metal.FilesystemLayout{ID: "fsl-1", Name: "fsl-1", Description: "FSL 1"}
	pgFilesystemLayout2 = &metal.FilesystemLayout{ID: "fsl-2", Name: "fsl-2", Description: "FSL 2"}
	pgFilesystemLayouts = []*metal.FilesystemLayout{pgFilesystemLayout1, pgFilesystemLayout2}

	pgEvent1 = &metal.ProvisioningEventContainer{ID: "m1"}
	pgEvent2 = &metal.ProvisioningEventContainer{ID: "m2"}
	pgEvents = []*metal.ProvisioningEventContainer{pgEvent1, pgEvent2}
)

// queryData runs a pg query and returns the decoded entity data, or nil when
// there are no matches, so it can be compared against the reference test
// expectations directly.
func queryData[T any](t *testing.T, ctx context.Context, repo *pg.GenericRepository[T], filters []pg.QueryFilter) []T {
	t.Helper()
	ents, err := repo.Query(ctx, filters, nil)
	require.NoError(t, err)
	got := make([]T, 0, len(ents))
	for _, e := range ents {
		got = append(got, e.Data)
	}
	if len(got) == 0 {
		return nil
	}
	return got
}

// TestEntityFiltersPostgres exercises the Postgres query builders for every
// entity that gained one alongside the port extraction, mirroring the
// RethinkDB-backed filter tests in pkg/db/queries.
func TestEntityFiltersPostgres(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	ctx := t.Context()

	t.Run("IP", func(t *testing.T) {
		repo, err := pg.NewGenericRepository[*metal.IP](log, db)
		require.NoError(t, err)
		for _, ip := range pgIPs {
			require.NoError(t, repo.Create(ctx, ip.GetID(), ip))
		}

		tests := []struct {
			name string
			rq   *apiv2.IPQuery
			want []*metal.IP
		}{
			{name: "empty request returns unfiltered", rq: nil, want: pgIPs},
			{name: "by ip", rq: &apiv2.IPQuery{Ip: &pgIP1.IPAddress}, want: []*metal.IP{pgIP1}},
			{name: "by uuid", rq: &apiv2.IPQuery{Uuid: &pgIP1.AllocationUUID}, want: []*metal.IP{pgIP1}},
			{name: "by name", rq: &apiv2.IPQuery{Name: &pgIP5.Name}, want: []*metal.IP{pgIP5}},
			{name: "by project", rq: &apiv2.IPQuery{Project: &pgIP1.ProjectID}, want: []*metal.IP{pgIP1, pgIP2, pgIP3, pgIP5}},
			{name: "by project different", rq: &apiv2.IPQuery{Project: &pgIP4.ProjectID}, want: []*metal.IP{pgIP4}},
			{name: "by namespace", rq: &apiv2.IPQuery{Namespace: pgIP3.Namespace}, want: []*metal.IP{pgIP3}},
			{name: "by network", rq: &apiv2.IPQuery{Network: &pgIP1.NetworkID}, want: []*metal.IP{pgIP1, pgIP4}},
			{name: "by parent prefix cidr", rq: &apiv2.IPQuery{ParentPrefixCidr: &pgIP1.ParentPrefixCidr}, want: []*metal.IP{pgIP1, pgIP2, pgIP4}},
			{name: "by label", rq: &apiv2.IPQuery{Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}}, want: []*metal.IP{pgIP1}},
			{name: "by label 2", rq: &apiv2.IPQuery{Labels: &apiv2.Labels{Labels: map[string]string{"color": "green"}}}, want: []*metal.IP{pgIP2}},
			{name: "by machine", rq: &apiv2.IPQuery{Machine: new("m1")}, want: []*metal.IP{pgIP1}},
			{name: "by machine empty result", rq: &apiv2.IPQuery{Machine: new("nonexistent-machine")}, want: nil},
			{name: "by type ephemeral", rq: &apiv2.IPQuery{Type: apiv2.IPType_IP_TYPE_EPHEMERAL.Enum()}, want: []*metal.IP{pgIP1, pgIP3, pgIP5}},
			{name: "by type static", rq: &apiv2.IPQuery{Type: apiv2.IPType_IP_TYPE_STATIC.Enum()}, want: []*metal.IP{pgIP2, pgIP4}},
			{name: "by address family v4", rq: &apiv2.IPQuery{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.Enum()}, want: []*metal.IP{pgIP1, pgIP2, pgIP3, pgIP4}},
			{name: "by address family v6", rq: &apiv2.IPQuery{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum()}, want: []*metal.IP{pgIP5}},
			{name: "combined by project and type", rq: &apiv2.IPQuery{Project: &pgIP1.ProjectID, Type: apiv2.IPType_IP_TYPE_STATIC.Enum()}, want: []*metal.IP{pgIP2}},
			{name: "combined by ip and project", rq: &apiv2.IPQuery{Ip: &pgIP1.IPAddress, Project: &pgIP1.ProjectID}, want: []*metal.IP{pgIP1}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := queryData(t, ctx, repo, q.IpFilter(tt.rq))
				if diff := cmp.Diff(
					tt.want, got,
					cmpopts.IgnoreFields(metal.IP{}, "Created", "Changed"),
					cmpopts.SortSlices(func(a, b *metal.IP) bool { return a.IPAddress < b.IPAddress }),
				); diff != "" {
					t.Errorf("IpFilter() = %v, want %v diff: %s", got, tt.want, diff)
				}
			})
		}

		got := queryData(t, ctx, repo, q.IpProjectScoped("project-2"))
		require.Len(t, got, 1)
		require.Equal(t, pgIP4.IPAddress, got[0].IPAddress)
	})

	t.Run("Size", func(t *testing.T) {
		repo, err := pg.NewGenericRepository[*metal.Size](log, db)
		require.NoError(t, err)
		for _, size := range pgSizes {
			require.NoError(t, repo.Create(ctx, size.GetID(), size))
		}

		tests := []struct {
			name string
			rq   *apiv2.SizeQuery
			want []*metal.Size
		}{
			{name: "empty request returns unfiltered", rq: nil, want: pgSizes},
			{name: "by id", rq: &apiv2.SizeQuery{Id: &pgSize1.ID}, want: []*metal.Size{pgSize1}},
			{name: "by name", rq: &apiv2.SizeQuery{Name: &pgSize3.Name}, want: []*metal.Size{pgSize3}},
			{name: "by description", rq: &apiv2.SizeQuery{Description: &pgSize4.Description}, want: []*metal.Size{pgSize4}},
			{name: "by label", rq: &apiv2.SizeQuery{Labels: &apiv2.Labels{Labels: map[string]string{"tier": "standard"}}}, want: []*metal.Size{pgSize1, pgSize3}},
			{name: "by label 2", rq: &apiv2.SizeQuery{Labels: &apiv2.Labels{Labels: map[string]string{"gpu": "nvidia"}}}, want: []*metal.Size{pgSize4}},
			{name: "no result by label", rq: &apiv2.SizeQuery{Labels: &apiv2.Labels{Labels: map[string]string{"tier": "nonexistent"}}}, want: nil},
			{name: "no result by name", rq: &apiv2.SizeQuery{Name: new("nonexistent")}, want: nil},
			{name: "label and name combined", rq: &apiv2.SizeQuery{Name: &pgSize1.Name, Labels: &apiv2.Labels{Labels: map[string]string{"tier": "standard"}}}, want: []*metal.Size{pgSize1}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := queryData(t, ctx, repo, q.SizeFilter(tt.rq))
				if diff := cmp.Diff(
					tt.want, got,
					cmpopts.SortSlices(func(a, b *metal.Size) bool { return a.ID < b.ID }),
					cmpopts.IgnoreFields(metal.Size{}, "Created", "Changed", "Generation"),
				); diff != "" {
					t.Errorf("SizeFilter() = %v, want %v diff: %s", got, tt.want, diff)
				}
			})
		}
	})

	t.Run("SizeReservation", func(t *testing.T) {
		repo, err := pg.NewGenericRepository[*metal.SizeReservation](log, db)
		require.NoError(t, err)
		for _, r := range pgSizeReservations {
			require.NoError(t, repo.Create(ctx, r.GetID(), r))
		}

		tests := []struct {
			name string
			rq   *apiv2.SizeReservationQuery
			want []*metal.SizeReservation
		}{
			{name: "empty request returns unfiltered", rq: nil, want: pgSizeReservations},
			{name: "by id", rq: &apiv2.SizeReservationQuery{Id: &pgSizeReservation1.ID}, want: []*metal.SizeReservation{pgSizeReservation1}},
			{name: "by size", rq: &apiv2.SizeReservationQuery{Size: &pgSizeReservation2.SizeID}, want: []*metal.SizeReservation{pgSizeReservation2}},
			{name: "by name", rq: &apiv2.SizeReservationQuery{Name: &pgSizeReservation1.Name}, want: []*metal.SizeReservation{pgSizeReservation1}},
			{name: "by description", rq: &apiv2.SizeReservationQuery{Description: &pgSizeReservation2.Description}, want: []*metal.SizeReservation{pgSizeReservation2}},
			{name: "by label", rq: &apiv2.SizeReservationQuery{Labels: &apiv2.Labels{Labels: map[string]string{"tier": "standard"}}}, want: []*metal.SizeReservation{pgSizeReservation1}},
			{name: "by project", rq: &apiv2.SizeReservationQuery{Project: &pgSizeReservation2.ProjectID}, want: []*metal.SizeReservation{pgSizeReservation2}},
			{name: "by partition", rq: &apiv2.SizeReservationQuery{Partition: new("partition-c")}, want: []*metal.SizeReservation{pgSizeReservation2}},
			{name: "no result by project", rq: &apiv2.SizeReservationQuery{Project: new("nonexistent")}, want: nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := queryData(t, ctx, repo, q.SizeReservationFilter(tt.rq))
				if diff := cmp.Diff(
					tt.want, got,
					cmpopts.SortSlices(func(a, b *metal.SizeReservation) bool { return a.ID < b.ID }),
					cmpopts.IgnoreFields(metal.SizeReservation{}, "Created", "Changed", "Generation"),
				); diff != "" {
					t.Errorf("SizeReservationFilter() = %v, want %v diff: %s", got, tt.want, diff)
				}
			})
		}
	})

	t.Run("SizeImageConstraint", func(t *testing.T) {
		repo, err := pg.NewGenericRepository[*metal.SizeImageConstraint](log, db)
		require.NoError(t, err)
		for _, sic := range pgSizeImageConstraints {
			require.NoError(t, repo.Create(ctx, sic.GetID(), sic))
		}

		tests := []struct {
			name string
			rq   *apiv2.SizeImageConstraintQuery
			want []*metal.SizeImageConstraint
		}{
			{name: "empty request returns unfiltered", rq: nil, want: pgSizeImageConstraints},
			{name: "by size", rq: &apiv2.SizeImageConstraintQuery{Size: &pgSizeImageConstraint1.ID}, want: []*metal.SizeImageConstraint{pgSizeImageConstraint1}},
			{name: "by name", rq: &apiv2.SizeImageConstraintQuery{Name: &pgSizeImageConstraint3.Name}, want: []*metal.SizeImageConstraint{pgSizeImageConstraint3}},
			{name: "by description", rq: &apiv2.SizeImageConstraintQuery{Description: &pgSizeImageConstraint4.Description}, want: []*metal.SizeImageConstraint{pgSizeImageConstraint4}},
			{name: "no result by size", rq: &apiv2.SizeImageConstraintQuery{Size: new("nonexistent")}, want: nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := queryData(t, ctx, repo, q.SizeImageConstraintFilter(tt.rq))
				if diff := cmp.Diff(
					tt.want, got,
					cmpopts.SortSlices(func(a, b *metal.SizeImageConstraint) bool { return a.ID < b.ID }),
					cmpopts.IgnoreFields(metal.SizeImageConstraint{}, "Created", "Changed", "Generation"),
				); diff != "" {
					t.Errorf("SizeImageConstraintFilter() = %v, want %v diff: %s", got, tt.want, diff)
				}
			})
		}
	})

	t.Run("Image", func(t *testing.T) {
		repo, err := pg.NewGenericRepository[*metal.Image](log, db)
		require.NoError(t, err)
		for _, img := range pgImages {
			require.NoError(t, repo.Create(ctx, img.GetID(), img))
		}

		tests := []struct {
			name string
			rq   *apiv2.ImageQuery
			want []*metal.Image
		}{
			{name: "empty request returns unfiltered", rq: nil, want: pgImages},
			{name: "by id", rq: &apiv2.ImageQuery{Id: &pgImage1.ID}, want: []*metal.Image{pgImage1}},
			{name: "by os", rq: &apiv2.ImageQuery{Os: &pgImage1.OS}, want: []*metal.Image{pgImage1, pgImage2, pgImage4}},
			{name: "by version", rq: &apiv2.ImageQuery{Version: &pgImage2.Version}, want: []*metal.Image{pgImage2}},
			{name: "by name", rq: &apiv2.ImageQuery{Name: &pgImage3.Name}, want: []*metal.Image{pgImage3}},
			{name: "by description", rq: &apiv2.ImageQuery{Description: &pgImage4.Description}, want: []*metal.Image{pgImage4}},
			{name: "by label", rq: &apiv2.ImageQuery{Labels: &apiv2.Labels{Labels: map[string]string{"a": "b"}}}, want: []*metal.Image{pgImage3}},
			{name: "by machine feature", rq: &apiv2.ImageQuery{Feature: apiv2.ImageFeature_IMAGE_FEATURE_MACHINE.Enum()}, want: []*metal.Image{pgImage1, pgImage2, pgImage3, pgImage4, pgImage6}},
			{name: "by firewall feature", rq: &apiv2.ImageQuery{Feature: apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL.Enum()}, want: []*metal.Image{pgImage3, pgImage5}},
			{name: "by supported classification", rq: &apiv2.ImageQuery{Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED.Enum()}, want: []*metal.Image{pgImage1, pgImage2, pgImage3, pgImage5}},
			{name: "by deprecated classification", rq: &apiv2.ImageQuery{Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_DEPRECATED.Enum()}, want: []*metal.Image{pgImage4}},
			{name: "by preview classification", rq: &apiv2.ImageQuery{Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW.Enum()}, want: []*metal.Image{pgImage6}},
			{name: "by os and version combined", rq: &apiv2.ImageQuery{Os: &pgImage1.OS, Version: &pgImage1.Version}, want: []*metal.Image{pgImage1}},
			{name: "by os and classification combined", rq: &apiv2.ImageQuery{Os: &pgImage1.OS, Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_PREVIEW.Enum()}, want: nil},
			{name: "by machine feature and ubuntu os", rq: &apiv2.ImageQuery{Os: &pgImage3.OS, Feature: apiv2.ImageFeature_IMAGE_FEATURE_MACHINE.Enum()}, want: []*metal.Image{pgImage3}},
			{name: "no result by wrong os", rq: &apiv2.ImageQuery{Os: new("nonexistent")}, want: nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := queryData(t, ctx, repo, q.ImageFilter(tt.rq))
				if diff := cmp.Diff(
					tt.want, got,
					cmpopts.SortSlices(func(a, b *metal.Image) bool { return a.ID < b.ID }),
					cmpopts.IgnoreFields(metal.Image{}, "Created", "Changed", "ExpirationDate"),
					cmpopts.IgnoreFields(metal.Base{}, "Created", "Changed", "Generation"),
				); diff != "" {
					t.Errorf("ImageFilter() = %v, want %v diff: %s", got, tt.want, diff)
				}
			})
		}
	})

	t.Run("Partition", func(t *testing.T) {
		repo, err := pg.NewGenericRepository[*metal.Partition](log, db)
		require.NoError(t, err)
		for _, p := range pgPartitions {
			require.NoError(t, repo.Create(ctx, p.GetID(), p))
		}

		tests := []struct {
			name string
			rq   *apiv2.PartitionQuery
			want []*metal.Partition
		}{
			{name: "empty request returns unfiltered", rq: nil, want: pgPartitions},
			{name: "by id", rq: &apiv2.PartitionQuery{Id: &pgPartition1.ID}, want: []*metal.Partition{pgPartition1}},
			{name: "no result by id", rq: &apiv2.PartitionQuery{Id: new("nonexistent")}, want: nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := queryData(t, ctx, repo, q.PartitionFilter(tt.rq))
				if diff := cmp.Diff(
					tt.want, got,
					cmpopts.SortSlices(func(a, b *metal.Partition) bool { return a.ID < b.ID }),
					cmpopts.IgnoreFields(metal.Partition{}, "Created", "Changed", "Generation"),
				); diff != "" {
					t.Errorf("PartitionFilter() = %v, want %v diff: %s", got, tt.want, diff)
				}
			})
		}
	})

	t.Run("Switch", func(t *testing.T) {
		repo, err := pg.NewGenericRepository[*metal.Switch](log, db)
		require.NoError(t, err)
		for _, sw := range pgSwitches {
			require.NoError(t, repo.Create(ctx, sw.GetID(), sw))
		}

		tests := []struct {
			name string
			rq   *apiv2.SwitchQuery
			want []*metal.Switch
		}{
			{name: "empty query returns all", rq: &apiv2.SwitchQuery{}, want: pgSwitches},
			{name: "query by id", rq: &apiv2.SwitchQuery{Id: new("sw1")}, want: []*metal.Switch{pgSwitch1}},
			{name: "query by partition", rq: &apiv2.SwitchQuery{Partition: new("partition-a")}, want: []*metal.Switch{pgSwitch1, pgSwitch3}},
			{name: "query by rack", rq: &apiv2.SwitchQuery{Rack: new("rack01")}, want: []*metal.Switch{pgSwitch1, pgSwitch2}},
			{name: "query by room", rq: &apiv2.SwitchQuery{Room: new("room01")}, want: []*metal.Switch{pgSwitch1}},
			{name: "query by os vendor", rq: &apiv2.SwitchQuery{Os: &apiv2.SwitchOSQuery{Vendor: new(apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC)}}, want: []*metal.Switch{pgSwitch3}},
			{name: "query by os version", rq: &apiv2.SwitchQuery{Os: &apiv2.SwitchOSQuery{Version: new("5.6")}}, want: []*metal.Switch{pgSwitch2}},
			{name: "query by connected machine id", rq: &apiv2.SwitchQuery{ConnectedMachineId: new("m1")}, want: []*metal.Switch{pgSwitch1, pgSwitch2}},
			{name: "query by connected machine id no result", rq: &apiv2.SwitchQuery{ConnectedMachineId: new("nonexistent")}, want: nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := queryData(t, ctx, repo, q.SwitchFilter(tt.rq))
				slices.SortFunc(got, func(a, b *metal.Switch) int { return strings.Compare(a.ID, b.ID) })
				slices.SortFunc(tt.want, func(a, b *metal.Switch) int { return strings.Compare(a.ID, b.ID) })
				if diff := cmp.Diff(
					tt.want, got,
					cmpopts.IgnoreFields(metal.Switch{}, "Created", "Changed"),
				); diff != "" {
					t.Errorf("SwitchFilter() = %v, want %v diff: %s", got, tt.want, diff)
				}
			})
		}
	})

	t.Run("FilesystemLayout", func(t *testing.T) {
		repo, err := pg.NewGenericRepository[*metal.FilesystemLayout](log, db)
		require.NoError(t, err)
		for _, f := range pgFilesystemLayouts {
			require.NoError(t, repo.Create(ctx, f.GetID(), f))
		}

		tests := []struct {
			name string
			rq   *apiv2.FilesystemServiceListRequest
			want []*metal.FilesystemLayout
		}{
			{name: "empty request returns unfiltered", rq: nil, want: pgFilesystemLayouts},
			{name: "by id", rq: &apiv2.FilesystemServiceListRequest{Id: &pgFilesystemLayout1.ID}, want: []*metal.FilesystemLayout{pgFilesystemLayout1}},
			{name: "no result by id", rq: &apiv2.FilesystemServiceListRequest{Id: new("nonexistent")}, want: nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := queryData(t, ctx, repo, q.FileSystemLayoutFilter(tt.rq))
				if diff := cmp.Diff(
					tt.want, got,
					cmpopts.SortSlices(func(a, b *metal.FilesystemLayout) bool { return a.ID < b.ID }),
					cmpopts.IgnoreFields(metal.FilesystemLayout{}, "Created", "Changed", "Generation"),
				); diff != "" {
					t.Errorf("FileSystemLayoutFilter() = %v, want %v diff: %s", got, tt.want, diff)
				}
			})
		}
	})

	t.Run("Event", func(t *testing.T) {
		repo, err := pg.NewGenericRepository[*metal.ProvisioningEventContainer](log, db)
		require.NoError(t, err)
		for _, e := range pgEvents {
			require.NoError(t, repo.Create(ctx, e.GetID(), e))
		}

		got := queryData(t, ctx, repo, q.EventFilter("m1"))
		require.Len(t, got, 1)
		require.Equal(t, pgEvent1.ID, got[0].ID)

		require.Nil(t, queryData(t, ctx, repo, q.EventFilter("nonexistent")))
	})
}
