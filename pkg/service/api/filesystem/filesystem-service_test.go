package filesystem

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_filesystemServiceServer_Match(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()
	dc := test.NewDatacenter(t, log)
	defer dc.Close()
	dc.Create(&sc.DefaultDatacenter)

	tests := []struct {
		name    string
		req     *apiv2.FilesystemServiceMatchRequest
		want    func(*test.Entities) *apiv2.FilesystemServiceMatchResponse
		wantErr error
	}{
		{
			name: "match a machine and fsl",
			req: &apiv2.FilesystemServiceMatchRequest{
				Match: &apiv2.FilesystemServiceMatchRequest_MachineAndFilesystemlayout{
					MachineAndFilesystemlayout: &apiv2.MatchMachine{
						Machine:          sc.Machine1,
						FilesystemLayout: "firewall",
					},
				},
			},
			want: func(e *test.Entities) *apiv2.FilesystemServiceMatchResponse {
				return &apiv2.FilesystemServiceMatchResponse{
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

			var want *apiv2.FilesystemServiceMatchResponse
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
