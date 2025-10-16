package infra

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
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	sw1 = &repository.SwitchServiceCreateRequest{
		Switch: &apiv2.Switch{
			Id:             "sw1",
			Description:    "switch 01",
			Rack:           pointer.Pointer("rack01"),
			Partition:      "partition-a",
			ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			ManagementIp:   "1.1.1.1",
			ManagementUser: pointer.Pointer("admin"),
			ConsoleCommand: pointer.Pointer("tty"),
			Os: &apiv2.SwitchOS{
				Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
				Version:          "ec202111",
				MetalCoreVersion: "v0.13.0",
			},
		},
	}
)

func Test_switchServiceServer_Register(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))
	defer ts.Close()

	validURL := ts.URL

	var (
		partitions = []*adminv2.PartitionServiceCreateRequest{
			{Partition: &apiv2.Partition{Id: "partition-a", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			{Partition: &apiv2.Partition{Id: "partition-b", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		}
	)

	test.CreatePartitions(t, repo, partitions)
	test.CreateSwitches(t, repo, []*repository.SwitchServiceCreateRequest{sw1})

	tests := []struct {
		name    string
		rq      *infrav2.SwitchServiceRegisterRequest
		want    *infrav2.SwitchServiceRegisterResponse
		wantErr error
	}{
		{
			name: "register new switch",
			rq: &infrav2.SwitchServiceRegisterRequest{
				Switch: &apiv2.Switch{
					Id:           "sw2",
					Rack:         nil,
					Partition:    "partition-b",
					ManagementIp: "1.1.1.1",
					ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					Nics:         []*apiv2.SwitchNic{},
					Os: &apiv2.SwitchOS{
						Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS,
						Version:          "v5.9",
						MetalCoreVersion: "v0.13.0",
					},
				},
			},
			want: &infrav2.SwitchServiceRegisterResponse{
				Switch: &apiv2.Switch{
					Id:           "sw2",
					Meta:         &apiv2.Meta{Generation: 0},
					Rack:         nil,
					Partition:    "partition-b",
					ManagementIp: "1.1.1.1",
					ReplaceMode:  apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					Nics:         []*apiv2.SwitchNic{},
					Os: &apiv2.SwitchOS{
						Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_CUMULUS,
						Version:          "v5.9",
						MetalCoreVersion: "v0.13.0",
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "register existing operational switch",
			rq: &infrav2.SwitchServiceRegisterRequest{
				Switch: &apiv2.Switch{
					Id:             "sw1",
					Description:    "new description",
					Partition:      "partition-a",
					ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					ManagementIp:   "1.1.1.1",
					ManagementUser: pointer.Pointer("admin"),
					ConsoleCommand: pointer.Pointer("tty"),
				},
			},
			want: &infrav2.SwitchServiceRegisterResponse{
				Switch: &apiv2.Switch{
					Id:             "sw1",
					Description:    "new description",
					Meta:           &apiv2.Meta{Generation: 1},
					Rack:           pointer.Pointer("rack01"),
					Partition:      "partition-a",
					ReplaceMode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
					ManagementIp:   "1.1.1.1",
					ManagementUser: pointer.Pointer("admin"),
					ConsoleCommand: pointer.Pointer("tty"),
					Os: &apiv2.SwitchOS{
						Vendor:           apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_SONIC,
						Version:          "ec202111",
						MetalCoreVersion: "v0.13.0",
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &switchServiceServer{
				log:  log,
				repo: repo,
			}

			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}

			got, err := s.Register(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("switchServiceServer.Register() error diff = %s", diff)
				return
			}
			if diff := cmp.Diff(tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("switchServiceServer.Register() diff = %s", diff)
			}
		})
	}
}
