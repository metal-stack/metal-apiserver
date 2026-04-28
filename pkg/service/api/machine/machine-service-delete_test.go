package machine

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	admintask "github.com/metal-stack/metal-apiserver/pkg/service/admin/task"
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

				m.RecentProvisioningEvents.Events = append([]*apiv2.MachineProvisioningEvent{{
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
						m1 := machines[sc.Machine1]

						m1.RecentProvisioningEvents.Events = append([]*apiv2.MachineProvisioningEvent{{
							Event:   apiv2.MachineProvisioningEventType_MACHINE_PROVISIONING_EVENT_TYPE_MACHINE_RECLAIM,
							Message: "reclaiming machine",
						}}, m1.RecentProvisioningEvents.Events...)

						m1.Allocation = nil
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

			var (
				m = &machineServiceServer{
					log:  log,
					repo: dc.GetTestStore().Store,
				}
				taskServer = admintask.New(admintask.Config{
					Log:  log,
					Repo: dc.GetTestStore().Store,
				})

				findTaskInList = func(taskType task.TaskType) (*adminv2.TaskInfo, error) {
					resp, err := taskServer.List(ctx, &adminv2.TaskServiceListRequest{})
					require.NoError(t, err)

					idx := slices.IndexFunc(resp.Tasks, func(info *adminv2.TaskInfo) bool {
						return info.Type == string(taskType)
					})
					if idx == -1 {
						return nil, fmt.Errorf("task not found")
					}

					return resp.Tasks[idx], nil
				}
			)

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

			// now wait for the bmc command to be triggered and fake a response

			var bmcPayload *task.MachineBMCCommandPayload

			require.Eventually(t, func() bool {
				tsk, err := findTaskInList(task.TypeMachineBMCCommand)
				if err != nil {
					return false
				}

				payload, err := task.DecodePayload[*task.MachineBMCCommandPayload](tsk.Payload)
				require.NoError(t, err)

				bmcPayload = payload

				return tsk.State == adminv2.TaskState_TASK_STATE_ACTIVE
			}, 5*time.Second, 1*time.Second, "machine delete bmc command task did not appear")

			_, err = dc.GetTestStore().UnscopedMachine().AdditionalMethods().BMCCommandDone(t.Context(), &infrav2.BMCCommandDoneRequest{
				CommandId: bmcPayload.CommandID,
			})
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				tsk, err := findTaskInList(task.TypeMachineBMCCommand)
				if err != nil {
					return false
				}

				payload, err := task.DecodePayload[*task.MachineBMCCommandPayload](tsk.Payload)
				require.NoError(t, err)

				bmcPayload = payload

				return tsk.State == adminv2.TaskState_TASK_STATE_COMPLETED
			}, 5*time.Second, 1*time.Second, "machine delete bmc command task did reach status completed")

			// the delete task should now be able to complete

			require.Eventually(t, func() bool {
				task, err := findTaskInList(task.TypeMachineDelete)
				if err != nil {
					return false
				}

				return task.State == adminv2.TaskState_TASK_STATE_COMPLETED
			}, 5*time.Second, 1*time.Second, "delete task did not reach status completed")

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
