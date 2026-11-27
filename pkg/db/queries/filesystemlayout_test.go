package queries_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/filters"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
)

var (
	fsl1 = &metal.FilesystemLayout{
		ID: "fsl1", Name: "layout 1", Description: "Filesystem layout 1",
		Filesystems:    []metal.Filesystem{},
		Disks:          []metal.Disk{},
		Raid:           []metal.Raid{},
		VolumeGroups:   []metal.VolumeGroup{},
		LogicalVolumes: metal.LogicalVolumes{},
		Constraints:    metal.FilesystemLayoutConstraints{Sizes: []string{"s1"}, Images: map[string]string{"debian": "12"}},
	}
	fsl2 = &metal.FilesystemLayout{
		ID: "fsl2", Name: "layout 2", Description: "Filesystem layout 2",
		Filesystems:    []metal.Filesystem{},
		Disks:          []metal.Disk{},
		Raid:           []metal.Raid{},
		VolumeGroups:   []metal.VolumeGroup{},
		LogicalVolumes: metal.LogicalVolumes{},
		Constraints:    metal.FilesystemLayoutConstraints{Sizes: []string{"s2"}, Images: map[string]string{"ubuntu": "24.04"}},
	}
	fsl3 = &metal.FilesystemLayout{
		ID: "fsl3", Name: "layout 3", Description: "Filesystem layout 3",
		Filesystems:    []metal.Filesystem{},
		Disks:          []metal.Disk{},
		Raid:           []metal.Raid{},
		VolumeGroups:   []metal.VolumeGroup{},
		LogicalVolumes: metal.LogicalVolumes{},
		Constraints:    metal.FilesystemLayoutConstraints{Sizes: []string{"s1", "s2"}, Images: map[string]string{"debian": "13"}},
	}
	filesystemLayouts = []*metal.FilesystemLayout{fsl1, fsl2, fsl3}
)

func TestFileSystemLayoutFilter(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ds, _, rethinkCloser := test.StartRethink(t, log)
	defer func() {
		rethinkCloser()
	}()

	ctx := t.Context()

	for _, fsl := range filesystemLayouts {
		created, err := ds.FilesystemLayout().Create(ctx, fsl)
		require.NoError(t, err)
		require.NotNil(t, created)
		require.Equal(t, fsl.ID, created.ID)
	}

	tests := []struct {
		name string
		rq   *apiv2.FilesystemServiceListRequest
		want []*metal.FilesystemLayout
	}{
		{
			name: "nil request returns unfiltered",
			rq:   nil,
			want: filesystemLayouts,
		},
		{
			name: "empty request returns unfiltered",
			rq:   &apiv2.FilesystemServiceListRequest{},
			want: filesystemLayouts,
		},
		{
			name: "by id",
			rq:   &apiv2.FilesystemServiceListRequest{Id: new("fsl1")},
			want: []*metal.FilesystemLayout{fsl1},
		},
		{
			name: "by id 2",
			rq:   &apiv2.FilesystemServiceListRequest{Id: new("fsl3")},
			want: []*metal.FilesystemLayout{fsl3},
		},
		{
			name: "no result by id",
			rq:   &apiv2.FilesystemServiceListRequest{Id: new("nonexistent")},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ds.FilesystemLayout().List(ctx, filters.FilesystemLayout(tt.rq)...)
			require.NoError(t, err)

			if diff := cmp.Diff(
				tt.want, got,
				cmpopts.SortSlices(func(a, b *metal.FilesystemLayout) bool { return a.ID < b.ID }),
				cmpopts.IgnoreFields(
					metal.FilesystemLayout{}, "Created", "Changed", "Generation",
				),
			); diff != "" {
				t.Errorf("FileSystemLayoutFilter() = %v, want %v, diff: %s", got, tt.want, diff)
			}
		})
	}
}
