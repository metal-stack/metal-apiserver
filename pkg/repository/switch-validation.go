package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/samber/lo"
)

func (r *switchRepository) validateCreate(ctx context.Context, req *SwitchServiceCreateRequest) error {
	var errs []error

	_, err := r.s.ds.Partition().Get(ctx, req.Switch.Partition)
	if err != nil {
		errs = append(errs, err)
	}

	sw, err := r.convertToInternal(req.Switch)
	if err != nil {
		errs = append(errs, err)
	}

	err = checkDuplicateNics(sw.Nics)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errorutil.NewInvalidArgument(errors.Join(errs...))
	}

	return nil
}

func (r *switchRepository) validateUpdate(ctx context.Context, req *adminv2.SwitchServiceUpdateRequest, oldSwitch *metal.Switch) error {
	var errs []error

	sw, err := r.s.ds.Switch().Get(ctx, req.Id)
	if err != nil {
		errs = append(errs, err)
	}

	_, err = r.s.ds.Partition().Get(ctx, sw.Partition)
	if err != nil {
		errs = append(errs, err)
	}

	err = checkDuplicateNics(sw.Nics)
	if err != nil {
		errs = append(errs, err)
	}

	// TODO: validate
	// - os vendor cannot be changed, only in replace or migrate

	reqNics, err := metal.ToMetalNics(req.Nics)
	if err != nil {
		errs = append(errs, err)
	}
	_, removedNics := lo.Difference(mapToIdentifier(reqNics), mapToIdentifier(sw.Nics))

	if connectedNics := connectedNics(removedNics, sw.MachineConnections); len(connectedNics) > 0 {
		errs = append(errs, fmt.Errorf("removing nics %v which are connected to a machine is not supported", connectedNics))
	}

	if err = validateNics(sw.Nics, reqNics, sw.MachineConnections); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errorutil.NewInvalidArgument(errors.Join(errs...))
	}

	return nil
}

func (r *switchRepository) validateDelete(ctx context.Context, sw *metal.Switch) error {
	panic("unimplemented")
}

func (r *switchRepository) validateReplace(ctx context.Context, old, new *apiv2.Switch) error {
	panic("unimplemented")
}

func checkDuplicateNics(nics metal.Nics) error {
	var errs []error

	duplicateIdentifiers := lo.FindDuplicates(mapToIdentifier(nics))
	duplicateNames := lo.FindDuplicates(mapToName(nics))

	if len(duplicateIdentifiers) > 0 {
		errs = append(errs, fmt.Errorf("switch nics contain duplicate identifiers:%v", duplicateIdentifiers))
	}
	if len(duplicateNames) > 0 {
		errs = append(errs, fmt.Errorf("switch nics contain duplicate name:%v", duplicateNames))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func connectedNics(nicIdentifiers []string, connections metal.ConnectionMap) []string {
	var connectedNics []string
	for _, nic := range nicIdentifiers {
		flatConnections := lo.Flatten(lo.Values(connections))
		connectionIdentifiers := lo.Map(flatConnections, func(c metal.Connection, i int) string {
			return c.Nic.Identifier
		})
		if slices.Contains(connectionIdentifiers, nic) {
			connectedNics = append(connectedNics, nic)
		}
	}
	return connectedNics
}

func validateNics(old, new metal.Nics, connections metal.ConnectionMap) error {
	var (
		errs                       []error
		cannotRemove, cannotRename []string
	)

	oldNics := old.MapByIdentifier()
	newNics := new.MapByIdentifier()

	for id, oldNic := range oldNics {
		if !nicIsConnected(id, connections) {
			continue
		}

		newNic, ok := newNics[id]
		if !ok {
			cannotRemove = append(cannotRemove, id)
			continue
		}

		if newNic != nil && newNic.Name != oldNic.Name {
			cannotRename = append(cannotRename, id)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func nicIsConnected(identifier string, connections metal.ConnectionMap) bool {
	flatConnections := lo.Flatten(lo.Values(connections))
	connectionIdentifiers := lo.Map(flatConnections, func(c metal.Connection, i int) string {
		return c.Nic.Identifier
	})
	if slices.Contains(connectionIdentifiers, identifier) {
		return true
	}
	return false
}

func mapToIdentifier(nics metal.Nics) []string {
	return lo.Map(nics, func(n metal.Nic, i int) string {
		return n.Identifier
	})
}

func mapToName(nics metal.Nics) []string {
	return lo.Map(nics, func(n metal.Nic, i int) string {
		return n.Name
	})
}
