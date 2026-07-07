package machine

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/api/go/errorutil"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	admintask "github.com/metal-stack/metal-apiserver/pkg/service/admin/task"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
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
					IPs: func(ips map[string]*apiv2.IP) {
						delete(ips, "12.110.0.1")
						delete(ips, "1.2.3.2")
					},
					Networks: func(networks map[string]*apiv2.Network) {
						var tenantNetwork *apiv2.Network

						for _, nw := range networks {
							if nw.Name != nil && *nw.Name == sc.NetworkNameTenantPartition1 {
								tenantNetwork = nw
								break
							}
						}

						require.NotNil(t, tenantNetwork, "tenant network was not created")

						networks[tenantNetwork.Id].Consumption.Ipv4.UsedIps--
						networks[sc.NetworkInternet].Consumption.Ipv4.UsedIps--
					},
				}
			},
		},
		{
			name: "not allocated, arbitrary machine is not found",
			rq: func(e *test.Entities) *apiv2.MachineServiceDeleteRequest {
				return &apiv2.MachineServiceDeleteRequest{
					Uuid:    e.Machines[sc.Machine2].Uuid,
					Project: e.Machines[sc.Machine1].Allocation.Project,
				}
			},
			wantErr: errorutil.NotFound(`*metal.Machine with id "00000000-0000-0000-0000-000000000002" not found`),
		},
	}

	dc := test.NewDatacenter(t, log, test.WithPostgres(true))
	defer dc.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.Create(&sc.DatacenterWithAllocations)
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

			var g errgroup.Group

			g.Go(func() error {
				if tt.wantErr != nil {
					return nil
				}

				// wait for the bmc command to be triggered and fake a response
				var bmcPayload *task.MachineBMCCommandPayload

				err := retry.Do(func() error {
					tsk, err := findTaskInList(task.TypeMachineBMCCommand)
					if err != nil {
						return err
					}

					payload, err := task.DecodePayload[*task.MachineBMCCommandPayload](tsk.Payload)
					if err != nil {
						return err
					}

					bmcPayload = payload

					if tsk.State != adminv2.TaskState_TASK_STATE_ACTIVE {
						return fmt.Errorf("task state not yet reached")
					}

					return nil
				}, retry.Attempts(5), retry.Delay(1*time.Second), retry.Context(t.Context()))
				if err != nil {
					return err
				}

				if _, err = dc.GetTestStore().UnscopedMachine().AdditionalMethods().BMCCommandDone(t.Context(), &infrav2.BMCCommandDoneRequest{
					CommandId: bmcPayload.CommandID,
				}); err != nil {
					return err
				}

				err = retry.Do(func() error {
					tsk, err := findTaskInList(task.TypeMachineBMCCommand)
					if err != nil {
						return err
					}

					payload, err := task.DecodePayload[*task.MachineBMCCommandPayload](tsk.Payload)
					if err != nil {
						return err
					}

					bmcPayload = payload

					log.Info("task poll in bmc simulation", "state", tsk.State.String())

					if tsk.State != adminv2.TaskState_TASK_STATE_COMPLETED {
						return fmt.Errorf("task state not yet reached")
					}

					return nil
				}, retry.Attempts(5), retry.Delay(1*time.Second), retry.Context(t.Context()))
				if err != nil {
					return err
				}

				log.Info("bmc simulation done")

				return nil
			})

			var (
				waitErr  error
				waitDone = make(chan (bool))
			)

			go func() {
				waitErr = g.Wait()
				waitDone <- true
			}()

			got, err := m.Delete(ctx, rq)
			if diff := cmp.Diff(tt.wantErr, err, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("error diff = %s", diff)
				return
			}

			if tt.wantErr != nil {
				err = dc.Assert(nil,
					protocmp.IgnoreFields(
						&apiv2.MachineProvisioningEvent{}, "time",
					),
					protocmp.IgnoreFields(
						&apiv2.MachineRecentProvisioningEvents{}, "last_event_time",
					),
				)
				require.NoError(t, err)

				return
			}

			if diff := cmp.Diff(want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at", "generation", "deletion_task_id",
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

			log.Info("waiting for bmc sim to finish")
			<-waitDone
			require.NoError(t, waitErr)

			if tt.wantErr != nil {
				return
			}

			// check also that delete task has completed

			require.NotNil(t, got.Machine.Meta)
			require.NotNil(t, got.Machine.Meta.DeletionTaskId)

			task, err := taskServer.Get(ctx, &adminv2.TaskServiceGetRequest{
				TaskId: *got.Machine.Meta.DeletionTaskId,
				Queue:  "default",
			})
			require.NoError(t, err)

			assert.Equal(t, adminv2.TaskState_TASK_STATE_COMPLETED, task.Task.State)

		})
	}
}
