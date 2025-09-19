package admin

import (
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Test_filesystemServiceServer_Create(t *testing.T) {
	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

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
					Meta:        &apiv2.Meta{Generation: 0},
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
							"debian": ">=12.04",
						},
					},
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("given imageconstraint:>=12.04 is not valid, missing space between op and version? invalid semantic version"),
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
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("imageServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_filesystemServiceServer_Update(t *testing.T) {
	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	fslMap := test.CreateFilesystemLayouts(t, repo, []*adminv2.FilesystemServiceCreateRequest{
		{
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
	})

	log.Info("fslMap", "fsl", fslMap["default"])

	tests := []struct {
		name    string
		rq      *adminv2.FilesystemServiceUpdateRequest
		want    *adminv2.FilesystemServiceUpdateResponse
		wantErr error
	}{
		{
			name: "update constraints",
			rq: &adminv2.FilesystemServiceUpdateRequest{
				Id: "default",
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(fslMap["default"].Changed),
				},
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
			want: &adminv2.FilesystemServiceUpdateResponse{
				FilesystemLayout: &apiv2.FilesystemLayout{
					Id:          "default",
					Meta:        &apiv2.Meta{Generation: 1},
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
			rq:      &adminv2.FilesystemServiceUpdateRequest{Id: "no-existing"},
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
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("imageServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_filesystemServiceServer_Delete(t *testing.T) {
	log := slog.Default()
	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	test.CreateFilesystemLayouts(t, repo, []*adminv2.FilesystemServiceCreateRequest{
		{
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
		{
			FilesystemLayout: &apiv2.FilesystemLayout{
				Id:          "m1-large",
				Name:        pointer.Pointer("Default FSL"),
				Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: pointer.Pointer("/"), Label: pointer.Pointer("root")}},
				Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: pointer.Pointer("1")}}}},
				Constraints: &apiv2.FilesystemLayoutConstraints{
					Sizes: []string{"m1-large-x86"},
					Images: map[string]string{
						"debian": ">= 12.0",
					},
				},
			},
		},
	})

	test.CreateMachines(t, testStore, []*metal.Machine{
		{
			Base: metal.Base{ID: "m1"}, PartitionID: "partition-one",
			Allocation: &metal.MachineAllocation{
				FilesystemLayout: &metal.FilesystemLayout{
					Base: metal.Base{ID: "m1-large"},
				},
			},
		},
	})

	tests := []struct {
		name    string
		rq      *adminv2.FilesystemServiceDeleteRequest
		want    *adminv2.FilesystemServiceDeleteResponse
		wantErr error
	}{
		{
			name: "delete existing",
			rq: &adminv2.FilesystemServiceDeleteRequest{
				Id: "default",
			},
			want: &adminv2.FilesystemServiceDeleteResponse{
				FilesystemLayout: &apiv2.FilesystemLayout{
					Id:          "default",
					Meta:        &apiv2.Meta{Generation: 0},
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
		{
			name:    "remove with existing machine allocation",
			rq:      &adminv2.FilesystemServiceDeleteRequest{Id: "m1-large"},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`cannot remove filesystemlayout with existing machine allocations`),
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
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("imageServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}
