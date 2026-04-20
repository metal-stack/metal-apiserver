package repository

import (
	"context"
	"errors"
	"fmt"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	sizeReservationRepository struct {
		s     *Store
		scope *ProjectScope
	}
)

func (r *sizeReservationRepository) get(ctx context.Context, id string) (*metal.SizeReservation, error) {
	sizeReservation, err := r.s.ds.SizeReservation().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return sizeReservation, nil
}

func (r *sizeReservationRepository) matchScope(sizeReservation *metal.SizeReservation) bool {
	if r.scope == nil {
		return true
	}

	return r.scope.projectID == pointer.SafeDeref(sizeReservation).ProjectID
}

func (r *sizeReservationRepository) create(ctx context.Context, req *adminv2.SizeReservationServiceCreateRequest) (*metal.SizeReservation, error) {
	if req.SizeReservation == nil {
		return nil, nil
	}
	sizeReservation, err := r.convertToInternal(ctx, req.SizeReservation)
	if err != nil {
		return nil, err
	}

	resp, err := r.s.ds.SizeReservation().Create(ctx, sizeReservation)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (r *sizeReservationRepository) update(ctx context.Context, e *metal.SizeReservation, req *adminv2.SizeReservationServiceUpdateRequest) (*metal.SizeReservation, error) {
	if req.Description != nil {
		e.Description = *req.Description
	}
	if req.Name != nil {
		e.Name = *req.Name
	}
	if req.Amount != nil {
		e.Amount = int(*req.Amount)
	}
	if len(req.Partitions) != 0 {
		e.PartitionIDs = req.Partitions
	}
	if req.Labels != nil {
		e.Labels = updateLabelsOnMap(req.Labels, e.Labels)
	}

	err := r.s.ds.SizeReservation().Update(ctx, e)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func (r *sizeReservationRepository) delete(ctx context.Context, e *metal.SizeReservation) error {
	err := r.s.ds.SizeReservation().Delete(ctx, e)
	if err != nil {
		return err
	}

	return nil
}

func (r *sizeReservationRepository) find(ctx context.Context, rq *apiv2.SizeReservationQuery) (*metal.SizeReservation, error) {
	sizeReservation, err := r.s.ds.SizeReservation().Find(ctx, r.sizeReservationFilters(queries.SizeReservationFilter(rq))...)
	if err != nil {
		return nil, err
	}

	return sizeReservation, nil
}

func (r *sizeReservationRepository) list(ctx context.Context, rq *apiv2.SizeReservationQuery) ([]*metal.SizeReservation, error) {
	sizeReservations, err := r.s.ds.SizeReservation().List(ctx, r.sizeReservationFilters(queries.SizeReservationFilter(rq))...)
	if err != nil {
		return nil, err
	}

	return sizeReservations, nil
}

func (r *sizeReservationRepository) convertToInternal(ctx context.Context, e *apiv2.SizeReservation) (*metal.SizeReservation, error) {
	if e == nil {
		return nil, nil
	}
	var labels map[string]string
	if e.Meta != nil && e.Meta.Labels != nil {
		labels = e.Meta.Labels.Labels
	}

	sizeReservation := &metal.SizeReservation{
		Base: metal.Base{
			ID:          e.Id,
			Name:        e.Name,
			Description: e.Description,
		},
		Labels:       labels,
		SizeID:       e.Size,
		Amount:       int(e.Amount),
		ProjectID:    e.Project,
		PartitionIDs: e.Partitions,
	}

	return sizeReservation, nil
}

func (r *sizeReservationRepository) convertToProto(ctx context.Context, e *metal.SizeReservation) (*apiv2.SizeReservation, error) {
	if e == nil {
		return nil, errors.New("size reservation is nil")
	}

	var (
		labels *apiv2.Labels
	)

	if e.Labels != nil {
		labels = &apiv2.Labels{
			Labels: e.Labels,
		}
	}

	return &apiv2.SizeReservation{
		Id:          e.ID,
		Name:        e.Name,
		Description: e.Description,
		Project:     e.ProjectID,
		Partitions:  e.PartitionIDs,
		Size:        e.SizeID,
		Amount:      int32(e.Amount),
		Meta: &apiv2.Meta{
			Labels:     labels,
			CreatedAt:  timestamppb.New(e.Created),
			UpdatedAt:  timestamppb.New(e.Changed),
			Generation: e.Generation,
		},
	}, nil
}

func (r *sizeReservationRepository) sizeReservationFilters(filter generic.EntityQuery) []generic.EntityQuery {
	var qs []generic.EntityQuery
	if filter != nil {
		qs = append(qs, filter)
	}

	return qs
}

func (r *sizeReservationRepository) check(ctx context.Context, partition, project, size string) error {
	r.s.log.Debug("check", "partition", partition, "project", project, "size", size)

	reservations, err := r.list(ctx, &apiv2.SizeReservationQuery{
		Partition: &partition,
		Project:   &project,
		Size:      &size,
	})
	if err != nil {
		return err
	}

	candidates, err := r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{
		Partition:    &partition,
		Size:         &size,
		State:        apiv2.MachineState_MACHINE_STATE_AVAILABLE.Enum(), // Machines which are locked or reserved are not considered
		Waiting:      new(true),
		Preallocated: new(false),
		NotAllocated: new(true),
	}))
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return fmt.Errorf("no machine candidates available")
	}

	// FIXME is this required, as we only select machines which are in waiting ?
	ecs, err := r.s.ds.Event().List(ctx, nil)
	if err != nil {
		return err
	}
	ecMap := metal.ProvisioningEventsByID(ecs)

	var available []*metal.Machine
	for _, m := range candidates {
		ec, ok := ecMap[m.ID]
		if !ok {
			r.s.log.Error("cannot find machine provisioning event container", "machine", m, "error", err)
			// fall through, so the rest of the machines is getting evaluated
			continue
		}
		if ec.Liveliness != metal.MachineLivelinessAlive {
			continue
		}
		available = append(available, m)
	}
	if len(available) == 0 {
		return fmt.Errorf("no alive machine available")
	}

	partitionMachines, err := r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{
		Partition: &partition,
		Size:      &size,
	}))
	if err != nil {
		return err
	}
	var (
		machinesByProject = make(map[string][]*metal.Machine)
	)
	for _, m := range partitionMachines {
		if m.Allocation == nil {
			continue
		}
		machinesByProject[m.Allocation.Project] = append(machinesByProject[m.Allocation.Project], m)
	}

	ok := r.checkSizeReservations(available, project, machinesByProject, reservations)
	if ok {
		return nil
	}
	return fmt.Errorf("no machine available")
}

// checkSizeReservations returns true when an allocation is possible and
// false when size reservations prevent the allocation for the given project in the given partition
// FIXME only machine of project are considered, therefor only machines of the project must be passed
func (r *sizeReservationRepository) checkSizeReservations(available []*metal.Machine, projectid string, machinesByProject map[string][]*metal.Machine, reservations []*metal.SizeReservation) bool {
	if len(reservations) == 0 {
		return true
	}

	if len(available) == 0 {
		return false
	}

	var (
		amount = 0
	)

	for _, r := range reservations {
		// sum up the amount of reservations
		amount += r.Amount

		alreadyAllocated := len(machinesByProject[r.ProjectID])

		if projectid == r.ProjectID && alreadyAllocated < r.Amount {
			// allow allocation for the project when it has a reservation and there are still allocations left
			return true
		}

		// subtract already used up reservations of the project
		amount = max(amount-alreadyAllocated, 0)
	}

	return amount < len(available)
}
