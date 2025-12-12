package infra

import (
	"log/slog"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Test_eventServiceServer_Send(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()
	now := time.Now()

	test.CreateMachines(t, testStore, []*metal.Machine{
		{Base: metal.Base{ID: "m1"}},
	})

	tests := []struct {
		name                string
		rq                  *infrav2.EventServiceSendRequest
		want                *infrav2.EventServiceSendResponse
		wantEventContainers []*metal.ProvisioningEventContainer
		wantErr             error
	}{
		{
			name: "one machine sending event",
			rq: &infrav2.EventServiceSendRequest{
				Events: map[string]*infrav2.MachineProvisioningEvent{
					"m1": {
						Time:    timestamppb.New(now),
						Event:   infrav2.ProvisioningEventType_PROVISIONING_EVENT_TYPE_PXE_BOOTING,
						Message: "Machine is PXE booting.",
					},
				},
			},
			want: &infrav2.EventServiceSendResponse{
				Events: 1,
				Failed: []string{},
			},
			wantEventContainers: []*metal.ProvisioningEventContainer{
				{
					Base: metal.Base{
						ID:         "m1",
						Generation: 1,
					},
					Liveliness: metal.MachineLivelinessAlive,
					Events: metal.ProvisioningEvents{
						{
							Event:   metal.ProvisioningEventPXEBooting,
							Message: "Machine is PXE booting.",
						},
					},
					LastErrorEvent:       nil,
					CrashLoop:            false,
					FailedMachineReclaim: false,
				},
			},
			wantErr: nil,
		},
		// TODO: when machine create is implemented add a test where an event for an unknown machine is sent.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &eventServiceServer{
				log:  log,
				repo: repo,
			}

			if tt.wantErr == nil {
				test.Validate(t, tt.rq)
			}

			got, err := s.Send(ctx, tt.rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("eventServiceServer.Send() error diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("eventServiceServer.Send() = %v, want %v", got, tt.want)
			}

			ecDiffs := map[string]string{}
			for _, wantContainer := range tt.wantEventContainers {
				ec := testStore.GetEventContainer(wantContainer.ID)
				if diff := cmp.Diff(wantContainer, ec,
					cmpopts.IgnoreFields(
						metal.Base{}, "Created", "Changed",
					),
					cmpopts.IgnoreFields(
						metal.ProvisioningEventContainer{}, "LastEventTime",
					),
					cmpopts.IgnoreFields(
						metal.ProvisioningEvent{}, "Time",
					),
				); diff != "" {
					ecDiffs[wantContainer.ID] = diff
				}
			}

			if len(ecDiffs) > 0 {
				t.Errorf("eventServiceServer.Send() some event containers do not look as expected: %v", ecDiffs)
			}
		})
	}
}
