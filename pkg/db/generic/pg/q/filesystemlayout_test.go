package q_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg/q"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

var (
	pgFSL1 = &metal.FilesystemLayout{
		ID: "fsl1", Name: "layout 1", Description: "Filesystem layout 1",
		Filesystems:    []metal.Filesystem{},
		Disks:          []metal.Disk{},
		Raid:           []metal.Raid{},
		VolumeGroups:   []metal.VolumeGroup{},
		LogicalVolumes: metal.LogicalVolumes{},
		Constraints:    metal.FilesystemLayoutConstraints{Sizes: []string{"s1"}, Images: map[string]string{"debian": "12"}},
	}
	pgFSL2 = &metal.FilesystemLayout{
		ID: "fsl2", Name: "layout 2", Description: "Filesystem layout 2",
		Filesystems:    []metal.Filesystem{},
		Disks:          []metal.Disk{},
		Raid:           []metal.Raid{},
		VolumeGroups:   []metal.VolumeGroup{},
		LogicalVolumes: metal.LogicalVolumes{},
		Constraints:    metal.FilesystemLayoutConstraints{Sizes: []string{"s2"}, Images: map[string]string{"ubuntu": "24.04"}},
	}
	pgFSL3 = &metal.FilesystemLayout{
		ID: "fsl3", Name: "layout 3", Description: "Filesystem layout 3",
		Filesystems:    []metal.Filesystem{},
		Disks:          []metal.Disk{},
		Raid:           []metal.Raid{},
		VolumeGroups:   []metal.VolumeGroup{},
		LogicalVolumes: metal.LogicalVolumes{},
		Constraints:    metal.FilesystemLayoutConstraints{Sizes: []string{"s1", "s2"}, Images: map[string]string{"debian": "13"}},
	}
	pgFSLs = []*metal.FilesystemLayout{pgFSL1, pgFSL2, pgFSL3}
)

// TestFileSystemLayoutFilterPostgres mirrors the RethinkDB-backed
// TestFileSystemLayoutFilter from pkg/db/queries, but runs against the Postgres
// backend using q.FileSystemLayoutFilter.
func TestFileSystemLayoutFilterPostgres(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db, closer := test.StartPostgres(t, log)
	defer closer()

	repo, err := pg.NewGenericRepository[*metal.FilesystemLayout](log, db)
	require.NoError(t, err)

	ctx := t.Context()

	for _, fsl := range pgFSLs {
		require.NoError(t, repo.Create(ctx, fsl.GetID(), fsl))
	}

	tests := []struct {
		name string
		rq   *apiv2.FilesystemServiceListRequest
		want []*metal.FilesystemLayout
	}{
		{name: "nil request returns unfiltered", rq: nil, want: pgFSLs},
		{name: "empty request returns unfiltered", rq: &apiv2.FilesystemServiceListRequest{}, want: pgFSLs},
		{name: "by id", rq: &apiv2.FilesystemServiceListRequest{Id: new("fsl1")}, want: []*metal.FilesystemLayout{pgFSL1}},
		{name: "by id 2", rq: &apiv2.FilesystemServiceListRequest{Id: new("fsl3")}, want: []*metal.FilesystemLayout{pgFSL3}},
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
}
