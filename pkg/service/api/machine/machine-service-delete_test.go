package machine

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_machineServiceServer_DeleteMachine(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	// Add token to be able to get the user from the context
	testToken := apiv2.Token{
		User:      "unit-test-user",
		AdminRole: apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
	}
	ctx = token.ContextWithToken(ctx, &testToken)

	tests := []struct {
		name    string
		rq      func(e *test.Entities) *apiv2.MachineServiceDeleteRequest
		want    func(e *test.Entities) *apiv2.MachineServiceDeleteResponse
		mods    func() *test.Asserters
		wantErr error
	}{
		{
			name: "delete a machine",
			rq: func(e *test.Entities) *apiv2.MachineServiceDeleteRequest {
				return &apiv2.MachineServiceDeleteRequest{
					Uuid:    e.Machines[sc.Machine1].Uuid,
					Project: e.Machines[sc.Machine1].Allocation.Project,
				}
			},
			want: func(e *test.Entities) *apiv2.MachineServiceDeleteResponse {
				m := e.Machines[sc.Machine1]

				m.RecentProvisioningEvents.Events = append([]*apiv2.MachineProvisioningEvent{&apiv2.MachineProvisioningEvent{
					Event:   apiv2.MachineProvisioningEventType_MACHINE_PROVISIONING_EVENT_TYPE_MACHINE_RECLAIM,
					Message: "reclaiming machine",
				}}, m.RecentProvisioningEvents.Events...)

				return &apiv2.MachineServiceDeleteResponse{
					Machine: m,
				}
			},
			mods: func() *test.Asserters {
				return &test.Asserters{
					Machines: func(machines map[string]*apiv2.Machine) {
						machines[sc.Machine1].RecentProvisioningEvents.Events = append([]*apiv2.MachineProvisioningEvent{&apiv2.MachineProvisioningEvent{
							Event:   apiv2.MachineProvisioningEventType_MACHINE_PROVISIONING_EVENT_TYPE_MACHINE_RECLAIM,
							Message: "reclaiming machine",
						}}, machines[sc.Machine1].RecentProvisioningEvents.Events...)
					},
				}
			},
		},
	}

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.Create(&sc.DefaultDatacenter)
			defer dc.Cleanup()

			var (
				rq   *apiv2.MachineServiceDeleteRequest
				want *apiv2.MachineServiceDeleteResponse
			)

			if tt.rq != nil {
				rq = tt.rq(dc.Snapshot())
			}
			if tt.want != nil {
				want = tt.want(dc.Snapshot())
			}

			m := &machineServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			if tt.wantErr == nil {
				test.Validate(t, rq)
			}

			got, err := m.Delete(ctx, rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("error diff = %s", diff)
				return
			}

			if diff := cmp.Diff(want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at", "generation",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineProvisioningEvent{}, "time",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineRecentProvisioningEvents{}, "last_event_time",
				),
			); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			var mods *test.Asserters
			if tt.mods != nil {
				mods = tt.mods()
			}
			err = dc.Assert(mods,
				protocmp.IgnoreFields(
					&apiv2.MachineProvisioningEvent{}, "time",
				),
				protocmp.IgnoreFields(
					&apiv2.MachineRecentProvisioningEvents{}, "last_event_time",
				),
			)
			require.NoError(t, err)
		})
	}
}
