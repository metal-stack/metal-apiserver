package admin

import (
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/Masterminds/semver/v3"
	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_filesystemServiceServer_Create(t *testing.T) {
	// Restore old (<= v3.3.1) behavior
	semver.CoerceNewVersion = false

	log := slog.Default()
	repo, closer := test.StartRepository(t, log)
	defer closer()

	tests := []struct {
		name    string
		rq      *adminv2.FilesystemServiceCreateRequest
		want    *adminv2.FilesystemServiceCreateResponse
		wantErr error
	}{
		{
			name: "create simple",
			rq: &adminv2.FilesystemServiceCreateRequest{
				FilesystemLayout: &apiv2.FilesystemLayout{
					Id:          "default",
					Name:        pointer.Pointer("Default FSL"),
					Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: pointer.Pointer("/"), Label: pointer.Pointer("root")}},
					Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: pointer.Pointer("1")}}}},
					Constraints: &apiv2.FilesystemLayoutConstraints{
						Sizes: []string{"c1-large-x86"},
						Images: map[string]string{
							"debian": ">= 12.0",
						},
					},
				},
			},
			want: &adminv2.FilesystemServiceCreateResponse{
				FilesystemLayout: &apiv2.FilesystemLayout{
					Id:          "default",
					Name:        pointer.Pointer("Default FSL"),
					Description: pointer.Pointer(""),
					Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: pointer.Pointer("/"), Label: pointer.Pointer("root")}},
					Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: pointer.Pointer("1")}}}},
					Constraints: &apiv2.FilesystemLayoutConstraints{
						Sizes: []string{"c1-large-x86"},
						Images: map[string]string{
							"debian": ">= 12.0",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "invalid constraint",
			rq: &adminv2.FilesystemServiceCreateRequest{
				FilesystemLayout: &apiv2.FilesystemLayout{
					Id:          "default",
					Name:        pointer.Pointer("Default FSL"),
					Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: pointer.Pointer("/"), Label: pointer.Pointer("root")}},
					Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: pointer.Pointer("1")}}}},
					Constraints: &apiv2.FilesystemLayoutConstraints{
						Sizes: []string{"c1-large-x86"},
						Images: map[string]string{
							"debian": ">= 12.04",
						},
					},
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("given version:12.04 is not valid:version segment starts with 0"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &filesystemServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := f.Create(t.Context(), connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("imageServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_filesystemServiceServer_Update(t *testing.T) {
	log := slog.Default()
	repo, closer := test.StartRepository(t, log)
	defer closer()

	f := &filesystemServiceServer{repo: repo, log: log}

	fslcr := &adminv2.FilesystemServiceCreateRequest{
		FilesystemLayout: &apiv2.FilesystemLayout{
			Id:          "default",
			Name:        pointer.Pointer("Default FSL"),
			Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: pointer.Pointer("/"), Label: pointer.Pointer("root")}},
			Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: pointer.Pointer("1")}}}},
			Constraints: &apiv2.FilesystemLayoutConstraints{
				Sizes: []string{"c1-large-x86"},
				Images: map[string]string{
					"debian": ">= 12.0",
				},
			},
		},
	}

	existing, err := f.Create(t.Context(), connect.NewRequest(fslcr))
	require.NoError(t, err)

	tests := []struct {
		name    string
		rq      *adminv2.FilesystemServiceUpdateRequest
		want    *adminv2.FilesystemServiceUpdateResponse
		wantErr error
	}{
		{
			name: "update constraints",
			rq: &adminv2.FilesystemServiceUpdateRequest{
				FilesystemLayout: &apiv2.FilesystemLayout{
					Id:          existing.Msg.FilesystemLayout.Id,
					Name:        pointer.Pointer("Default FSL"),
					Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: pointer.Pointer("/"), Label: pointer.Pointer("root")}},
					Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: pointer.Pointer("1")}}}},
					Constraints: &apiv2.FilesystemLayoutConstraints{
						Sizes: []string{"c1-large-x86"},
						Images: map[string]string{
							"debian": ">= 12.0",
							"ubuntu": ">= 24.4",
						},
					},
				},
			},
			want: &adminv2.FilesystemServiceUpdateResponse{
				FilesystemLayout: &apiv2.FilesystemLayout{
					Id:          "default",
					Name:        pointer.Pointer("Default FSL"),
					Description: pointer.Pointer(""),
					Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: pointer.Pointer("/"), Label: pointer.Pointer("root")}},
					Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: pointer.Pointer("1")}}}},
					Constraints: &apiv2.FilesystemLayoutConstraints{
						Sizes: []string{"c1-large-x86"},
						Images: map[string]string{
							"debian": ">= 12.0",
							"ubuntu": ">= 24.4",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name:    "update nonexisting",
			rq:      &adminv2.FilesystemServiceUpdateRequest{FilesystemLayout: &apiv2.FilesystemLayout{Id: "no-existing"}},
			want:    nil,
			wantErr: errorutil.NotFound(`no filesystemlayout with id "no-existing" found`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &filesystemServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := f.Update(t.Context(), connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("imageServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_filesystemServiceServer_Delete(t *testing.T) {
	log := slog.Default()
	repo, closer := test.StartRepository(t, log)
	defer closer()

	f := &filesystemServiceServer{repo: repo, log: log}

	fslcr := &adminv2.FilesystemServiceCreateRequest{
		FilesystemLayout: &apiv2.FilesystemLayout{
			Id:          "default",
			Name:        pointer.Pointer("Default FSL"),
			Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: pointer.Pointer("/"), Label: pointer.Pointer("root")}},
			Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: pointer.Pointer("1")}}}},
			Constraints: &apiv2.FilesystemLayoutConstraints{
				Sizes: []string{"c1-large-x86"},
				Images: map[string]string{
					"debian": ">= 12.0",
				},
			},
		},
	}

	existing, err := f.Create(t.Context(), connect.NewRequest(fslcr))
	require.NoError(t, err)

	tests := []struct {
		name    string
		rq      *adminv2.FilesystemServiceDeleteRequest
		want    *adminv2.FilesystemServiceDeleteResponse
		wantErr error
	}{
		{
			name: "delete existing",
			rq: &adminv2.FilesystemServiceDeleteRequest{
				Id: existing.Msg.FilesystemLayout.Id,
			},
			want: &adminv2.FilesystemServiceDeleteResponse{
				FilesystemLayout: &apiv2.FilesystemLayout{
					Id:          "default",
					Name:        pointer.Pointer("Default FSL"),
					Description: pointer.Pointer(""),
					Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: pointer.Pointer("/"), Label: pointer.Pointer("root")}},
					Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: pointer.Pointer("1")}}}},
					Constraints: &apiv2.FilesystemLayoutConstraints{
						Sizes: []string{"c1-large-x86"},
						Images: map[string]string{
							"debian": ">= 12.0",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name:    "remove now not existing anymore",
			rq:      &adminv2.FilesystemServiceDeleteRequest{Id: "default"},
			want:    nil,
			wantErr: errorutil.NotFound(`no filesystemlayout with id "default" found`),
		},
		{
			name:    "remove nonexisting",
			rq:      &adminv2.FilesystemServiceDeleteRequest{Id: "no-existing"},
			want:    nil,
			wantErr: errorutil.NotFound(`no filesystemlayout with id "no-existing" found`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &filesystemServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := f.Delete(t.Context(), connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("imageServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}
