package housekeeping

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	admintask "github.com/metal-stack/metal-apiserver/pkg/service/admin/task"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"github.com/stretchr/testify/require"
)

func Test_machineServiceServer_MachineLiveliness(t *testing.T) {
	t.Parallel()

	dc := test.NewDatacenter(t)
	defer dc.Close()

	testDC := sc.DefaultDatacenter

	machines := []*sc.MachineWithLiveliness{
		{
			Machine:    &metal.Machine{ID: sc.Machine2, PartitionID: sc.Partition1, SizeID: sc.SizeC1Large},
			Liveliness: metal.MachineLivelinessAlive,
		},
		{
			Machine:    &metal.Machine{ID: sc.Machine3, PartitionID: sc.Partition1, SizeID: sc.SizeC1Large, Allocation: &metal.MachineAllocation{Project: sc.Tenant1Project1, ImageID: sc.ImageDebian13}},
			Liveliness: metal.MachineLivelinessAlive,
		},
		{
			Machine:    &metal.Machine{ID: sc.Machine4, PartitionID: sc.Partition1, SizeID: sc.SizeC1Large},
			Liveliness: metal.MachineLivelinessAlive,
		},
	}

	testDC.Machines = append(testDC.Machines, machines...)
	dc.Create(&testDC)

	var (
		recent = time.Now().Add(-1 * time.Minute) // younger than metal.MachineDeadAfter
		old    = time.Now().Add(-1 * time.Hour)   // older than metal.MachineDeadAfter
	)

	setLastEventTime := func(id string, lastEventTime *time.Time) {
		ec := dc.GetTestStore().GetEventContainer(id)
		ec.LastEventTime = lastEventTime
		err := dc.GetTestStore().GetDatastore().Event().Update(t.Context(), ec)
		require.NoError(t, err)
	}

	// m1: recent event -> alive
	setLastEventTime(sc.Machine1, &recent)
	// m2: stale event, not allocated -> dead
	setLastEventTime(sc.Machine2, &old)
	// m3: stale event, allocated -> unknown, because the customer may have turned off phone home
	setLastEventTime(sc.Machine3, &old)
	// m4: no event time at all -> liveliness must be left untouched

	hk := &housekeepingServiceServer{
		repo: dc.GetTestStore().Store,
		log:  dc.GetTestStore().GetLogger(),
	}

	modifiedMachines, err := hk.MachineLiveliness(t.Context(), &adminv2.HousekeepingServiceMachineLivelinessRequest{})
	require.NoError(t, err)
	require.NotNil(t, modifiedMachines)
	require.Len(t, modifiedMachines.ModifiedMachines, 2)

	require.Equal(t, metal.MachineLivelinessAlive, dc.GetTestStore().GetEventContainer(sc.Machine1).Liveliness)
	require.Equal(t, metal.MachineLivelinessDead, dc.GetTestStore().GetEventContainer(sc.Machine2).Liveliness)
	require.Equal(t, metal.MachineLivelinessUnknown, dc.GetTestStore().GetEventContainer(sc.Machine3).Liveliness)
	require.Equal(t, metal.MachineLivelinessAlive, dc.GetTestStore().GetEventContainer(sc.Machine4).Liveliness)
}

func Test_machineServiceServer_Resurrect(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	// older than metal.MachineResurrectAfter so that a dead machine qualifies for resurrection
	resurrectableLastEventTime := time.Now().Add(-2 * time.Hour)

	tests := []struct {
		name            string
		configureEC     func(ec *metal.ProvisioningEventContainer)
		wantResurrected bool
	}{
		{
			name: "resurrects a dead machine",
			configureEC: func(ec *metal.ProvisioningEventContainer) {
				ec.Liveliness = metal.MachineLivelinessDead
				ec.LastEventTime = &resurrectableLastEventTime
			},
			wantResurrected: true,
		},
		{
			name: "resurrects a machine with failed machine reclaim",
			configureEC: func(ec *metal.ProvisioningEventContainer) {
				ec.FailedMachineReclaim = true
				ec.LastEventTime = &resurrectableLastEventTime
			},
			wantResurrected: true,
		},
		{
			name: "does not resurrect an alive machine",
			configureEC: func(ec *metal.ProvisioningEventContainer) {
				ec.Liveliness = metal.MachineLivelinessAlive
				ec.LastEventTime = &resurrectableLastEventTime
			},
			wantResurrected: false,
		},
		{
			name: "does not resurrect a machine without last event time",
			configureEC: func(ec *metal.ProvisioningEventContainer) {
				ec.Liveliness = metal.MachineLivelinessDead
			},
			wantResurrected: false,
		},
	}

	dc := test.NewDatacenter(t)
	log := dc.GetTestStore().GetLogger()
	defer dc.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.Create(&sc.DatacenterWithAllocations)
			defer dc.Cleanup()

			testStore := dc.GetTestStore()

			// Machine2 is not allocated, so it is the one that is evaluated for resurrection.
			ec := testStore.GetEventContainer(sc.Machine2)
			tt.configureEC(ec)
			err := testStore.GetDatastore().Event().Update(ctx, ec)
			require.NoError(t, err)

			var (
				simErr  error
				simDone chan struct{}
			)

			if tt.wantResurrected {
				simDone = make(chan struct{})

				// The decommission workflow triggers a bmc command which needs to be acknowledged for it to complete.
				taskServer := admintask.New(admintask.Config{
					Log:  log,
					Repo: testStore.Store,
				})
				findActiveBMCCommandTask := func() (*adminv2.TaskInfo, error) {
					resp, err := taskServer.List(ctx, &adminv2.TaskServiceListRequest{})
					if err != nil {
						return nil, err
					}

					idx := slices.IndexFunc(resp.Tasks, func(info *adminv2.TaskInfo) bool {
						return info.Type == string(task.TypeMachineBMCCommand) && info.State == adminv2.TaskState_TASK_STATE_ACTIVE
					})
					if idx == -1 {
						return nil, fmt.Errorf("no active bmc command task found")
					}

					return resp.Tasks[idx], nil
				}

				go func() {
					defer close(simDone)

					var bmcPayload *task.MachineBMCCommandPayload

					err := retry.Do(func() error {
						tsk, err := findActiveBMCCommandTask()
						if err != nil {
							return err
						}

						payload, err := task.DecodePayload[*task.MachineBMCCommandPayload](tsk.Payload)
						if err != nil {
							return err
						}

						bmcPayload = payload
						return nil
					}, retry.Attempts(10), retry.Delay(1*time.Second), retry.Context(ctx))
					if err != nil {
						simErr = err
						return
					}

					if _, err = testStore.UnscopedMachine().AdditionalMethods().BMCCommandDone(ctx, &infrav2.BMCCommandDoneRequest{
						CommandId: bmcPayload.CommandID,
					}); err != nil {
						simErr = err
					}
				}()
			}

			hk := &housekeepingServiceServer{
				repo: dc.GetTestStore().Store,
				log:  dc.GetTestStore().GetLogger(),
			}
			modifiedMachines, err := hk.MachineResurrect(t.Context(), &adminv2.HousekeepingServiceMachineResurrectRequest{})
			require.NoError(t, err)
			require.NotNil(t, modifiedMachines)

			if tt.wantResurrected {
				<-simDone
				require.NoError(t, simErr)
			}

			ec = testStore.GetEventContainer(sc.Machine2)
			hasReclaimEvent := slices.ContainsFunc(ec.Events, func(e metal.ProvisioningEvent) bool {
				return e.Event == metal.ProvisioningEventMachineReclaim
			})
			require.Equal(t, tt.wantResurrected, hasReclaimEvent, "unexpected machine reclaim event state")
		})
	}
}
