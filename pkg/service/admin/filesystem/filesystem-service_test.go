package admin

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
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_filesystemServiceServer_Create(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
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
					Id:   "default",
					Name: new("Default FSL"),
					Filesystems: []*apiv2.Filesystem{
						{
							Device:        "/dev/sda1",
							Format:        apiv2.Format_FORMAT_EXT4,
							Path:          new("/"),
							Label:         new("root"),
							MountOptions:  []string{"defaults", "noatime"},
							CreateOptions: []string{"-L", "root"},
						},
					},
					Disks: []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: new("1"), GptType: apiv2.GPTType_GPT_TYPE_LINUX.Enum()}}}},
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
					Name:        new("Default FSL"),
					Description: new(""),
					Filesystems: []*apiv2.Filesystem{
						{
							Device:        "/dev/sda1",
							Format:        apiv2.Format_FORMAT_EXT4,
							Path:          new("/"),
							Label:         new("root"),
							MountOptions:  []string{"defaults", "noatime"},
							CreateOptions: []string{"-L", "root"},
						},
					},
					Disks: []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: new("1"), GptType: apiv2.GPTType_GPT_TYPE_LINUX.Enum()}}}},
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
					Name:        new("Default FSL"),
					Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: new("/"), Label: new("root")}},
					Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: new("1"), GptType: apiv2.GPTType_GPT_TYPE_LINUX.Enum()}}}},
					Constraints: &apiv2.FilesystemLayoutConstraints{
						Sizes: []string{"c1-large-x86"},
						Images: map[string]string{
							"debian": ">=12.04",
						},
					},
				},
			},
			want:    nil,
			wantErr: errorutil.InvalidArgument("given imageconstraint:>=12.04 is not valid (missing space between op and version?): invalid semantic version"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &filesystemServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := f.Create(t.Context(), tt.rq)
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
				t.Errorf("filesystemServiceServer.Create() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_filesystemServiceServer_Update(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

	fslMap := test.CreateFilesystemLayouts(t, testStore, []*adminv2.FilesystemServiceCreateRequest{
		{
			FilesystemLayout: &apiv2.FilesystemLayout{
				Id:          "default",
				Name:        new("Default FSL"),
				Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: new("/"), Label: new("root")}},
				Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: new("1"), GptType: apiv2.GPTType_GPT_TYPE_LINUX.Enum()}}}},
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
					UpdatedAt: fslMap["default"].Meta.UpdatedAt,
				},
				Name:        new("Default FSL"),
				Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: new("/"), Label: new("root")}},
				Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: new("1"), GptType: apiv2.GPTType_GPT_TYPE_LINUX.Enum()}}}},
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
					Name:        new("Default FSL"),
					Description: new(""),
					Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: new("/"), Label: new("root")}},
					Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: new("1"), GptType: apiv2.GPTType_GPT_TYPE_LINUX.Enum()}}}},
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
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := f.Update(t.Context(), tt.rq)
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
				t.Errorf("filesystemServiceServer.Update() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_filesystemServiceServer_Delete(t *testing.T) {
	t.Parallel()

	log := slog.Default()
	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

	test.CreateFilesystemLayouts(t, testStore, []*adminv2.FilesystemServiceCreateRequest{
		{
			FilesystemLayout: &apiv2.FilesystemLayout{
				Id:          "default",
				Name:        new("Default FSL"),
				Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: new("/"), Label: new("root")}},
				Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: new("1"), GptType: apiv2.GPTType_GPT_TYPE_LINUX.Enum()}}}},
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
				Name:        new("Default FSL"),
				Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: new("/"), Label: new("root")}},
				Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: new("1")}}}},
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
					Name:        new("Default FSL"),
					Description: new(""),
					Filesystems: []*apiv2.Filesystem{{Device: "/dev/sda1", Format: apiv2.Format_FORMAT_EXT4, Path: new("/"), Label: new("root")}},
					Disks:       []*apiv2.Disk{{Device: "/dev/sda", Partitions: []*apiv2.DiskPartition{{Number: 1, Label: new("1"), GptType: apiv2.GPTType_GPT_TYPE_LINUX.Enum()}}}},
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
			wantErr: errorutil.FailedPrecondition(`cannot remove filesystemlayout with existing machine allocations`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &filesystemServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := f.Delete(t.Context(), tt.rq)
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
				t.Errorf("filesystemServiceServer.Delete() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_filesystemServiceServer_Match(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()
	dc := test.NewDatacenter(t, log)
	defer dc.Close()
	dc.Create(&sc.DefaultDatacenter)

	tests := []struct {
		name    string
		req     *adminv2.FilesystemServiceMatchRequest
		want    func(*test.Entities) *adminv2.FilesystemServiceMatchResponse
		wantErr error
	}{
		{
			name: "match a machine and fsl",
			req: &adminv2.FilesystemServiceMatchRequest{
				Match: &adminv2.FilesystemServiceMatchRequest_MachineAndFilesystemlayout{
					MachineAndFilesystemlayout: &adminv2.MatchMachineAndFilesystemLayout{
						Machine:          sc.Machine1,
						FilesystemLayout: "firewall",
					},
				},
			},
			want: func(e *test.Entities) *adminv2.FilesystemServiceMatchResponse {
				return &adminv2.FilesystemServiceMatchResponse{
					FilesystemLayout: &apiv2.FilesystemLayout{
						Id:          "firewall",
						Name:        new(""),
						Description: new(""),
						Meta:        &apiv2.Meta{},

						Constraints: &apiv2.FilesystemLayoutConstraints{
							Sizes: []string{sc.SizeN1Medium},
							Images: map[string]string{
								"firewall-ubuntu": ">= 3.0",
							},
						},
					},
				}
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &filesystemServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}

			var want *adminv2.FilesystemServiceMatchResponse
			if tt.want != nil {
				want = tt.want(dc.Snapshot())
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.req)
			}
			got, err := f.Match(ctx, tt.req)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("filesystemServiceServer.Match() diff: %s", diff)
			}
		})
	}
}

func Test_filesystemServiceServer_Try(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()
	dc := test.NewDatacenter(t, log)
	defer dc.Close()
	dc.Create(&sc.DefaultDatacenter)

	tests := []struct {
		name    string
		req     *adminv2.FilesystemServiceMatchRequest
		want    func(*test.Entities) *adminv2.FilesystemServiceMatchResponse
		wantErr error
	}{
		{
			name: "try size and image",
			req: &adminv2.FilesystemServiceMatchRequest{
				Match: &adminv2.FilesystemServiceMatchRequest_SizeAndImage{
					SizeAndImage: &adminv2.MatchImageAndSize{
						Size:  sc.SizeC1Large,
						Image: sc.ImageDebian12,
					},
				},
			},
			want: func(e *test.Entities) *adminv2.FilesystemServiceMatchResponse {
				return &adminv2.FilesystemServiceMatchResponse{
					FilesystemLayout: &apiv2.FilesystemLayout{
						Id:          "debian",
						Name:        new(""),
						Description: new(""),
						Meta:        &apiv2.Meta{},
						Disks: []*apiv2.Disk{
							{
								Device: "/dev/sda",
								Partitions: []*apiv2.DiskPartition{
									{
										Size: 1024,
									},
								},
							},
						},

						Constraints: &apiv2.FilesystemLayoutConstraints{
							Sizes: []string{sc.SizeC1Large, sc.SizeN1Medium},
							Images: map[string]string{
								"debian": ">= 12.0",
							},
						},
					},
				}
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &filesystemServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}

			var want *adminv2.FilesystemServiceMatchResponse
			if tt.want != nil {
				want = tt.want(dc.Snapshot())
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.req)
			}
			got, err := f.Match(ctx, tt.req)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("filesystemServiceServer.Match() diff: %s", diff)
			}
		})
	}
}
