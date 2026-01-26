package repository

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/samber/lo"
	"go4.org/netipx"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	switchRepository struct {
		s *Store
	}

	SwitchServiceCreateRequest struct {
		Switch *apiv2.Switch
	}

	SwitchStatus struct {
		ID            string
		LastSync      *apiv2.SwitchSync
		LastSyncError *apiv2.SwitchSync
	}
)

func (s *SwitchStatus) GetID() string {
	return s.ID
}

func (r *switchRepository) Register(ctx context.Context, req *infrav2.SwitchServiceRegisterRequest) (*apiv2.Switch, error) {
	if req == nil || req.Switch == nil {
		return nil, errorutil.InvalidArgument("empty request")
	}

	sw, err := r.get(ctx, req.Switch.Id)
	if err != nil && !errorutil.IsNotFound(err) {
		return nil, err
	}
	if errorutil.IsNotFound(err) {
		return r.s.Switch().Create(ctx, &SwitchServiceCreateRequest{Switch: req.Switch})
	}

	new := req.Switch
	old, err := r.convertToProto(ctx, sw)
	if err != nil {
		return nil, err
	}

	if sw.ReplaceMode == metal.SwitchReplaceModeReplace {
		sw, err := r.replace(ctx, old, new)
		if err != nil {
			return nil, err
		}

		converted, err := r.convertToProto(ctx, sw)
		if err != nil {
			return nil, err
		}

		return converted, nil
	}

	updateReq := &adminv2.SwitchServiceUpdateRequest{
		Id: new.Id,
		UpdateMeta: &apiv2.UpdateMeta{
			LockingStrategy: apiv2.OptimisticLockingStrategy_OPTIMISTIC_LOCKING_STRATEGY_SERVER,
		},
		Description:    pointer.PointerOrNil(new.Description),
		ReplaceMode:    pointer.PointerOrNil(new.ReplaceMode),
		ManagementIp:   pointer.PointerOrNil(new.ManagementIp),
		ManagementUser: new.ManagementUser,
		ConsoleCommand: new.ConsoleCommand,
		Nics:           new.Nics,
		Os:             new.Os,
	}

	err = r.validateUpdate(ctx, updateReq, sw)
	if err != nil {
		return nil, err
	}

	updated, err := r.updateOnRegister(ctx, sw, updateReq)
	if err != nil {
		return nil, err
	}

	converted, err := r.convertToProto(ctx, updated)
	if err != nil {
		return nil, err
	}

	return converted, nil
}

func (r *switchRepository) Migrate(ctx context.Context, oldSwitch, newSwitch string) (*apiv2.Switch, error) {
	if oldSwitch == "" {
		return nil, errorutil.InvalidArgument("old switch id cannot be empty")
	}

	if newSwitch == "" {
		return nil, errorutil.InvalidArgument("new switch id cannot be empty")
	}

	old, err := r.get(ctx, oldSwitch)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate switch %s to %s: %w", oldSwitch, newSwitch, err)
	}

	new, err := r.get(ctx, newSwitch)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate switch %s to %s: %w", oldSwitch, newSwitch, err)
	}

	if old.Rack != new.Rack {
		return nil, errorutil.FailedPrecondition("cannot migrate from switch %s in rack %s to switch %s in rack %s, switches must be in the same rack", oldSwitch, old.Rack, newSwitch, new.Rack)
	}

	if len(new.MachineConnections) > 0 {
		return nil, errorutil.FailedPrecondition("cannot migrate from switch %s to switch %s because the new switch already has machine connections", oldSwitch, newSwitch)
	}

	sw, err := adoptConfiguration(old, new)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate switch %s to %s: %w", oldSwitch, newSwitch, err)
	}

	err = r.migrateMachineConnections(ctx, old, sw)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate switch %s to %s: %w", oldSwitch, newSwitch, err)
	}

	err = r.s.ds.Switch().Update(ctx, sw)
	if err != nil {
		return nil, err
	}

	converted, err := r.convertToProto(ctx, sw)
	if err != nil {
		return nil, err
	}

	return converted, nil
}

func (r *switchRepository) Port(ctx context.Context, id, port string, status apiv2.SwitchPortStatus) (*apiv2.Switch, error) {
	metalStatus, err := metal.ToSwitchPortStatus(status)
	if err != nil {
		return nil, errorutil.InvalidArgument("failed to parse port status %q: %w", status, err)
	}

	if status != apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UP && status != apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_DOWN {
		return nil, errorutil.InvalidArgument("port status %q must be one of [%q, %q]", metalStatus, metal.SwitchPortStatusUp, metal.SwitchPortStatusDown)
	}

	sw, err := r.s.ds.Switch().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	nic, found := lo.Find(sw.Nics, func(nic metal.Nic) bool {
		return nic.Name == port
	})
	if !found {
		return nil, errorutil.InvalidArgument("port %s does not exist on switch %s", port, id)
	}

	m, err := r.getConnectedMachineForNic(ctx, nic, sw.MachineConnections)
	if err != nil {
		return nil, err
	}

	if m == nil {
		return nil, errorutil.FailedPrecondition("port %s is not connected to any machine", port)
	}

	nic.State.Desired = &metalStatus
	err = r.s.ds.Switch().Update(ctx, sw)
	if err != nil {
		return nil, err
	}

	converted, err := r.convertToProto(ctx, sw)
	if err != nil {
		return nil, err
	}

	return converted, nil
}

func (r *switchRepository) SearchSwitchesConnectedToMachine(ctx context.Context, m *metal.Machine) ([]*metal.Switch, error) {
	var switches []*metal.Switch

	rackSwitches, err := r.s.ds.Switch().List(ctx, queries.SwitchFilter(&apiv2.SwitchQuery{Rack: &m.RackID}))
	if err != nil {
		return nil, err
	}

	for _, sw := range rackSwitches {
		if _, has := sw.MachineConnections[m.ID]; has {
			switches = append(switches, sw)
		}
	}

	return switches, nil
}

func (r *switchRepository) SetVrfAtSwitches(ctx context.Context, m *metal.Machine, vrf string) ([]*metal.Switch, error) {
	var switches []*metal.Switch

	connectedSwitches, err := r.SearchSwitchesConnectedToMachine(ctx, m)
	if err != nil {
		return nil, err
	}

	for _, sw := range connectedSwitches {
		sw.SetVrfOfMachine(m, vrf)
		err := r.s.ds.Switch().Update(ctx, sw)
		if err != nil {
			return nil, err
		}
	}

	return switches, nil
}

func (r *switchRepository) ConnectMachineWithSwitches(ctx context.Context, m *apiv2.Machine) error {
	if m.Partition == nil || m.Partition.Id == "" {
		return errorutil.InvalidArgument("partition id of machine %s is empty", m.Uuid)
	}

	if m.Hardware == nil {
		return errorutil.InvalidArgument("no hardware information for machine %s given", m.Uuid)
	}

	neighs := lo.Uniq(lo.Flatten(
		lo.Map(m.Hardware.Nics, func(nic *apiv2.MachineNic, _ int) []string {
			return lo.Map(nic.Neighbors, func(neigh *apiv2.MachineNic, _ int) string {
				return neigh.Hostname
			})
		}),
	))

	if len(neighs) != 2 {
		return errorutil.FailedPrecondition("machine %s is not connected to exactly two switches, found connections to switches %v", m.Uuid, neighs)
	}

	metalMachine, err := r.s.ds.Machine().Get(ctx, m.Uuid)
	if err != nil && !errorutil.IsNotFound(err) {
		return err
	}

	if metalMachine != nil {
		oldNeighs := lo.Uniq(lo.Flatten(
			lo.Map(metalMachine.Hardware.Nics, func(nic metal.Nic, _ int) []string {
				return lo.Map(nic.Neighbors, func(neigh metal.Nic, _ int) string {
					return neigh.Hostname
				})
			}),
		))

		prev, _ := lo.Difference(oldNeighs, neighs)
		for _, id := range prev {
			s, err := r.get(ctx, id)
			if err != nil {
				return fmt.Errorf("failed to remove machine connection from switch %s: %w", id, err)
			}

			cons := s.MachineConnections
			delete(cons, m.Uuid)

			err = r.s.ds.Switch().Update(ctx, s)
			if err != nil {
				return fmt.Errorf("failed to remove machine connection from switch %s: %w", id, err)
			}
		}
	}

	s1, err := r.get(ctx, neighs[0])
	if err != nil {
		return fmt.Errorf("failed to add machine connections to switch %s: %w", neighs[0], err)
	}
	s2, err := r.get(ctx, neighs[1])
	if err != nil {
		return fmt.Errorf("failed to add machine connections to switch %s: %w", neighs[1], err)
	}

	if s1.Rack != s2.Rack {
		return errorutil.FailedPrecondition("connected switches of a machine must reside in the same rack, rack of switch %s: %s, rack of switch %s: %s, machine: %s", s1.Name, s1.Rack, s2.Name, s2.Rack, m.Uuid)
	}
	m.Rack = s1.Rack

	var newMachineNics metal.Nics
	for _, n := range m.Hardware.Nics {
		newNic := metal.Nic{}
		for _, neigh := range n.Neighbors {
			newNic.Neighbors = append(newNic.Neighbors, metal.Nic{
				Name:       neigh.Name,
				Identifier: neigh.Identifier,
				Hostname:   neigh.Hostname,
			})
		}
		newMachineNics = append(newMachineNics, newNic)
	}

	_, err = s1.ConnectMachine(m.Uuid, newMachineNics)
	if err != nil {
		return fmt.Errorf("failed to update machine connections for switch %s: %w", s1.ID, err)
	}
	_, err = s2.ConnectMachine(m.Uuid, newMachineNics)
	if err != nil {
		return fmt.Errorf("failed to update machine connections for switch %s: %w", s2.ID, err)
	}

	cons1 := s1.MachineConnections[m.Uuid]
	cons2 := s2.MachineConnections[m.Uuid]

	if len(cons1) != len(cons2) {
		return errorutil.FailedPrecondition("machine connections must be identical on both switches but machine %s has %d connections to switch %s and %d connections to switch %s", m.Uuid, len(cons1), s1.ID, len(cons2), s2.ID)
	}

	byNicName, err := s2.MachineConnections.ByNicName()
	if err != nil {
		return fmt.Errorf("failed to map machine connections of switch %s by nic names", s2.ID)
	}

	// e.g. "swp1s0" -> "Ethernet0"
	switchPortMapping, err := s1.MapPortNames(s2.OS.Vendor)
	if err != nil {
		return fmt.Errorf("could not create port mapping %w", err)
	}

	for _, con := range s1.MachineConnections[m.Uuid] {
		name, ok := switchPortMapping[con.Nic.Name]
		if !ok {
			return fmt.Errorf("could not translate port name %s to equivalent port name of switch os %s", con.Nic.Name, s1.OS.Vendor)
		}
		if _, has := byNicName[name]; !has {
			return errorutil.FailedPrecondition("machine %s is connected to port %s on switch %s but not to the corresponding port %s of switch %s", m.Uuid, con.Nic.Name, s1.ID, name, s2.ID)
		}
	}

	err = r.s.ds.Switch().Update(ctx, s1)
	if err != nil {
		return fmt.Errorf("failed to update machine connections for switch %s: %w", s1.ID, err)
	}
	err = r.s.ds.Switch().Update(ctx, s2)
	if err != nil {
		return fmt.Errorf("failed to update machine connections for switch %s: %w", s2.ID, err)
	}

	return nil
}

func (r *switchRepository) ForceDelete(ctx context.Context, switchID string) (*apiv2.Switch, error) {
	sw, err := r.get(ctx, switchID)
	if err != nil {
		return nil, err
	}

	err = r.delete(ctx, sw)
	if err != nil {
		return nil, err
	}

	converted, err := r.convertToProto(ctx, sw)
	if err != nil {
		return nil, err
	}

	return converted, nil
}

func (r *switchRepository) GetSwitchStatus(ctx context.Context, switchID string) (*SwitchStatus, error) {
	metalStatus, err := r.s.ds.SwitchStatus().Get(ctx, switchID)
	if err != nil && !errorutil.IsNotFound(err) {
		return nil, err
	}

	status := &SwitchStatus{
		ID: switchID,
	}

	if errorutil.IsNotFound(err) {
		return status, nil
	}

	if metalStatus.LastSync != nil {
		status.LastSync = &apiv2.SwitchSync{
			Time:     timestamppb.New(metalStatus.LastSync.Time),
			Duration: durationpb.New(metalStatus.LastSync.Duration),
			Error:    metalStatus.LastSync.Error,
		}
	}

	if metalStatus.LastSyncError != nil {
		status.LastSyncError = &apiv2.SwitchSync{
			Time:     timestamppb.New(metalStatus.LastSyncError.Time),
			Duration: durationpb.New(metalStatus.LastSyncError.Duration),
			Error:    metalStatus.LastSyncError.Error,
		}
	}

	return status, nil
}

func (r *switchRepository) SetSwitchStatus(ctx context.Context, status *SwitchStatus) error {
	metalStatus := &metal.SwitchStatus{
		Base: metal.Base{
			ID: status.ID,
		},
		LastSync:      toMetalSwitchSync(status.LastSync),
		LastSyncError: toMetalSwitchSync(status.LastSyncError),
	}

	return r.s.ds.SwitchStatus().Upsert(ctx, metalStatus)
}

func GetNewNicState(current *apiv2.NicState, status apiv2.SwitchPortStatus) (*apiv2.NicState, bool) {
	if current == nil {
		return &apiv2.NicState{
			Actual: status,
		}, true
	}

	state := &apiv2.NicState{
		Desired: current.Desired,
		Actual:  status,
	}

	if state.Desired != nil && status == *state.Desired {
		state.Desired = nil
	}

	changed := cmp.Diff(current, state, protocmp.Transform()) != ""
	return state, changed
}

func (r *switchRepository) get(ctx context.Context, id string) (*metal.Switch, error) {
	sw, err := r.s.ds.Switch().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return sw, nil
}

func toSwitchBGPPortState(state *apiv2.SwitchBGPPortState) (*metal.SwitchBGPPortState, error) {
	if state == nil {
		return nil, nil
	}

	bgpState, err := metal.ToBGPState(state.BgpState)
	if err != nil {
		return nil, err
	}

	bgpPortState := &metal.SwitchBGPPortState{
		Neighbor:              state.Neighbor,
		PeerGroup:             state.PeerGroup,
		VrfName:               state.VrfName,
		BgpState:              bgpState,
		BgpTimerUpEstablished: uint64(state.BgpTimerUpEstablished.Seconds),
		SentPrefixCounter:     state.SentPrefixCounter,
		AcceptedPrefixCounter: state.AcceptedPrefixCounter,
	}

	return bgpPortState, nil
}

func toSwitchPortState(state *apiv2.NicState) (*metal.NicState, error) {
	if state == nil {
		return nil, nil
	}

	var (
		actual  metal.SwitchPortStatus
		desired *metal.SwitchPortStatus
		err     error
	)

	if state.Desired != nil {
		portStatus, err := metal.ToSwitchPortStatus(*state.Desired)
		if err != nil {
			return nil, err
		}
		desired = &portStatus
	}
	actual, err = metal.ToSwitchPortStatus(state.Actual)
	if err != nil {
		return nil, err
	}

	return &metal.NicState{
		Desired: desired,
		Actual:  actual,
	}, nil
}

func (r *switchRepository) create(ctx context.Context, req *SwitchServiceCreateRequest) (*metal.Switch, error) {
	if req.Switch == nil {
		return nil, nil
	}
	sw, err := r.convertToInternal(ctx, req.Switch)
	if err != nil {
		return nil, err
	}

	resp, err := r.s.ds.Switch().Create(ctx, sw)
	if err != nil {
		return nil, err
	}

	r.s.log.Info("created switch in metal-db", "switch", sw)

	return resp, nil
}

func (r *switchRepository) update(ctx context.Context, sw *metal.Switch, req *adminv2.SwitchServiceUpdateRequest) (*metal.Switch, error) {
	if req == nil {
		return nil, errorutil.Internal("failed to update switch %s, request was empty", sw.ID)
	}

	updated, err := updateAllButNics(sw, req)
	if err != nil {
		return nil, err
	}

	if len(req.Nics) > 0 {
		nics, err := toMetalNics(req.Nics, sw.ID)
		if err != nil {
			return nil, err
		}
		updated.Nics = nics
	}

	err = r.s.ds.Switch().Update(ctx, sw)
	if err != nil {
		return nil, err
	}

	return sw, nil
}

func (r *switchRepository) delete(ctx context.Context, sw *metal.Switch) error {
	status, err := r.s.ds.SwitchStatus().Get(ctx, sw.ID)
	if err != nil && !errorutil.IsNotFound(err) {
		return err
	}
	err = r.s.ds.SwitchStatus().Delete(ctx, status)
	if err != nil {
		return err
	}
	return r.s.ds.Switch().Delete(ctx, sw)
}

func (r *switchRepository) replace(ctx context.Context, oldSwitch, newSwitch *apiv2.Switch) (*metal.Switch, error) {
	old, err := r.convertToInternal(ctx, oldSwitch)
	if err != nil {
		return nil, fmt.Errorf("failed to replace switch %s by %s: %w", oldSwitch.Id, newSwitch.Id, err)
	}
	new, err := r.convertToInternal(ctx, newSwitch)
	if err != nil {
		return nil, fmt.Errorf("failed to replace switch %s by %s: %w", oldSwitch.Id, newSwitch.Id, err)
	}
	twin, err := r.findTwinSwitch(ctx, new)
	if err != nil {
		return nil, fmt.Errorf("failed to determine twin for switch %s: %w", newSwitch.Id, err)
	}
	sw, err := adoptFromTwin(old, twin, new)
	if err != nil {
		return nil, fmt.Errorf("failed to adopt configuration from twin for switch %s, err: %w", newSwitch.Id, err)
	}
	nicMap, err := sw.TranslateNicMap(old.OS.Vendor)
	if err != nil {
		return nil, fmt.Errorf("failed to replace switch %s by %s: %w", oldSwitch.Id, newSwitch.Id, err)
	}
	err = r.adjustMachineConnections(ctx, old.MachineConnections, nicMap)
	if err != nil {
		return nil, fmt.Errorf("failed to replace switch %s by %s: %w", oldSwitch.Id, newSwitch.Id, err)
	}

	sw.SetChanged(oldSwitch.Meta.UpdatedAt.AsTime())
	err = r.s.ds.Switch().Update(ctx, sw)
	if err != nil {
		return nil, fmt.Errorf("failed to replace switch %s by %s: %w", oldSwitch.Id, newSwitch.Id, err)
	}

	return sw, nil
}

func (r *switchRepository) find(ctx context.Context, query *apiv2.SwitchQuery) (*metal.Switch, error) {
	sw, err := r.s.ds.Switch().Find(ctx, r.switchFilters(queries.SwitchFilter(query))...)
	if err != nil {
		return nil, err
	}
	return sw, nil
}

func (r *switchRepository) list(ctx context.Context, query *apiv2.SwitchQuery) ([]*metal.Switch, error) {
	switches, err := r.s.ds.Switch().List(ctx, r.switchFilters(queries.SwitchFilter(query))...)
	if err != nil {
		return nil, err
	}
	return switches, err
}

func (r *switchRepository) convertToInternal(ctx context.Context, sw *apiv2.Switch) (*metal.Switch, error) {
	if sw == nil {
		return nil, nil
	}

	nics, err := toMetalNics(sw.Nics, sw.Id)
	if err != nil {
		return nil, err
	}

	replaceMode, err := metal.ToReplaceMode(sw.ReplaceMode)
	if err != nil {
		return nil, err
	}

	if sw.Os == nil {
		return nil, errorutil.InvalidArgument("switch os for switch %s is empty", sw.Id)
	}

	vendor, err := metal.ToSwitchOSVendor(sw.Os.Vendor)
	if err != nil {
		return nil, err
	}

	connections, err := toMachineConnections(sw.MachineConnections, sw.Id)
	if err != nil {
		return nil, err
	}

	return &metal.Switch{
		Base: metal.Base{
			ID:          sw.Id,
			Name:        sw.Id,
			Description: sw.Description,
		},
		Rack:               pointer.SafeDeref(sw.Rack),
		Partition:          sw.Partition,
		ReplaceMode:        replaceMode,
		ManagementIP:       sw.ManagementIp,
		ManagementUser:     pointer.SafeDeref(sw.ManagementUser),
		ConsoleCommand:     pointer.SafeDeref(sw.ConsoleCommand),
		MachineConnections: connections,
		OS: &metal.SwitchOS{
			Vendor:           vendor,
			Version:          sw.Os.Version,
			MetalCoreVersion: sw.Os.MetalCoreVersion,
		},
		Nics: nics,
	}, nil
}

func (r *switchRepository) convertToProto(ctx context.Context, sw *metal.Switch) (*apiv2.Switch, error) {
	if sw == nil {
		return nil, nil
	}

	nics, err := r.convertToSwitchNics(ctx, sw)
	if err != nil {
		return nil, err
	}

	connections, err := convertMachineConnections(sw.MachineConnections, nics)
	if err != nil {
		return nil, err
	}

	replaceMode, err := metal.FromReplaceMode(sw.ReplaceMode)
	if err != nil {
		return nil, err
	}

	vendor, err := metal.FromSwitchOSVendor(sw.OS.Vendor)
	if err != nil {
		return nil, err
	}

	return &apiv2.Switch{
		Id: sw.ID,
		Meta: &apiv2.Meta{
			CreatedAt:  timestamppb.New(sw.Created),
			UpdatedAt:  timestamppb.New(sw.Changed),
			Generation: sw.Generation,
		},
		Description:        sw.Description,
		Rack:               pointer.PointerOrNil(sw.Rack),
		Partition:          sw.Partition,
		ReplaceMode:        replaceMode,
		ManagementIp:       sw.ManagementIP,
		ManagementUser:     pointer.PointerOrNil(sw.ManagementUser),
		ConsoleCommand:     pointer.PointerOrNil(sw.ConsoleCommand),
		Nics:               nics,
		MachineConnections: connections,
		Os: &apiv2.SwitchOS{
			Vendor:           vendor,
			Version:          sw.OS.Version,
			MetalCoreVersion: sw.OS.MetalCoreVersion,
		},
	}, nil
}

func (r *switchRepository) matchScope(sw *metal.Switch) bool {
	// switches are not project scoped
	return true
}

func (r *switchRepository) switchFilters(filter generic.EntityQuery) []generic.EntityQuery {
	var qs []generic.EntityQuery
	if filter != nil {
		qs = append(qs, filter)
	}
	return qs
}

func (r *switchRepository) findTwinSwitch(ctx context.Context, newSwitch *metal.Switch) (*metal.Switch, error) {
	rackSwitches, err := r.list(ctx, &apiv2.SwitchQuery{Rack: &newSwitch.Rack, Partition: &newSwitch.Partition})
	if err != nil {
		return nil, fmt.Errorf("failed to find twin switch for switch %s in rack %s", newSwitch.ID, newSwitch.Rack)
	}

	if len(rackSwitches) == 0 {
		return nil, errorutil.NotFound("could not find any switch in rack %v", newSwitch.Rack)
	}

	var twin *metal.Switch
	for i := range rackSwitches {
		sw := rackSwitches[i]
		if sw.ReplaceMode == metal.SwitchReplaceModeReplace || sw.ID == newSwitch.ID {
			continue
		}
		if len(sw.MachineConnections) == 0 {
			continue
		}
		if twin == nil {
			twin = sw
		} else {
			return nil, errorutil.FailedPrecondition("found multiple twin switches for %v (%v and %v)", newSwitch.ID, twin.ID, sw.ID)
		}
	}

	if twin == nil {
		return nil, errorutil.NotFound("no twin found for switch %s in partition %v and rack %v", newSwitch.ID, newSwitch.Partition, newSwitch.Rack)
	}

	return twin, nil
}

func (r *switchRepository) updateOnRegister(ctx context.Context, sw *metal.Switch, req *adminv2.SwitchServiceUpdateRequest) (*metal.Switch, error) {
	updated, err := updateAllButNics(sw, req)
	if err != nil {
		return nil, err
	}

	if len(req.Nics) > 0 {
		nics, err := toMetalNics(req.Nics, sw.ID)
		if err != nil {
			return nil, err
		}
		updated.Nics = updateNicNames(sw.Nics, nics)
	}

	err = r.s.ds.Switch().Update(ctx, sw)
	if err != nil {
		return nil, err
	}

	return sw, nil
}

func updateAllButNics(sw *metal.Switch, req *adminv2.SwitchServiceUpdateRequest) (*metal.Switch, error) {
	if req.Description != nil {
		sw.Description = *req.Description
	}
	if req.ReplaceMode != nil {
		replaceMode, err := metal.ToReplaceMode(*req.ReplaceMode)
		if err != nil {
			return nil, err
		}
		sw.ReplaceMode = replaceMode
	}
	if req.ManagementIp != nil {
		sw.ManagementIP = *req.ManagementIp
	}
	if req.ManagementUser != nil {
		sw.ManagementUser = *req.ManagementUser
	}
	if req.ConsoleCommand != nil {
		sw.ConsoleCommand = *req.ConsoleCommand
	}
	if req.Os != nil {
		vendor, err := metal.ToSwitchOSVendor(req.Os.Vendor)
		if err != nil {
			return nil, err
		}
		sw.OS = &metal.SwitchOS{
			Vendor:           vendor,
			Version:          req.Os.Version,
			MetalCoreVersion: req.Os.MetalCoreVersion,
		}
	}

	return sw, nil
}

func (r *switchRepository) convertToSwitchNics(ctx context.Context, sw *metal.Switch) ([]*apiv2.SwitchNic, error) {
	var (
		switchNics      []*apiv2.SwitchNic
		projectMachines []*metal.Machine
		desiredStatus   *apiv2.SwitchPortStatus
	)
	networks, err := r.s.ds.Network().List(ctx)
	if err != nil {
		return nil, err
	}

	ips, err := r.s.ds.IP().List(ctx)
	if err != nil {
		return nil, err
	}

	for _, nic := range sw.Nics {
		var bgpPortState *apiv2.SwitchBGPPortState
		if nic.BGPPortState != nil {
			bgpState, err := metal.FromBGPState(nic.BGPPortState.BgpState)
			if err != nil {
				return nil, err
			}

			bgpPortState = &apiv2.SwitchBGPPortState{
				Neighbor:              nic.BGPPortState.Neighbor,
				PeerGroup:             nic.BGPPortState.PeerGroup,
				VrfName:               nic.BGPPortState.VrfName,
				BgpState:              bgpState,
				BgpTimerUpEstablished: timestamppb.New(time.Unix(int64(nic.BGPPortState.BgpTimerUpEstablished), 0)),
				SentPrefixCounter:     nic.BGPPortState.SentPrefixCounter,
				AcceptedPrefixCounter: nic.BGPPortState.AcceptedPrefixCounter,
			}
		}

		if nic.State == nil {
			return nil, errorutil.InvalidArgument("nic state for port %s cannot be empty", nic.Name)
		}

		if nic.State.Desired != nil {
			portStatus, err := metal.FromSwitchPortStatus(nic.State.Desired)
			if err != nil {
				return nil, errorutil.InvalidArgument("failed to parse desired port status for nic %s: %w", nic.Name, err)
			}
			desiredStatus = &portStatus
		}
		actualStatus, err := metal.FromSwitchPortStatus(&nic.State.Actual)
		if err != nil {
			return nil, errorutil.InvalidArgument("failed to parse actual port status for nic %s: %w", nic.Name, err)
		}

		m, err := r.getConnectedMachineForNic(ctx, nic, sw.MachineConnections)
		if err != nil {
			return nil, err
		}

		if m != nil && m.Allocation != nil {
			projectMachines, err = r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{
				Allocation: &apiv2.MachineAllocationQuery{
					Project: pointer.Pointer(m.Allocation.Project),
				},
				Partition: pointer.Pointer(sw.Partition),
				Rack:      pointer.Pointer(sw.Rack),
			}))
			if err != nil {
				return nil, err
			}
		}

		filter, err := makeBGPFilter(m, projectMachines, nic.Vrf, networks, ips)
		if err != nil {
			return nil, err
		}

		switchNics = append(switchNics, &apiv2.SwitchNic{
			Name:       nic.Name,
			Identifier: nic.Identifier,
			Mac:        nic.MacAddress,
			Vrf:        pointer.PointerOrNil(nic.Vrf),
			State: &apiv2.NicState{
				Desired: desiredStatus,
				Actual:  actualStatus,
			},
			BgpFilter:    filter,
			BgpPortState: bgpPortState,
		})
	}

	slices.SortFunc(switchNics, func(n1, n2 *apiv2.SwitchNic) int {
		return strings.Compare(n1.Name, n2.Name)
	})

	return switchNics, nil
}

func (r *switchRepository) getConnectedMachineForNic(ctx context.Context, nic metal.Nic, connections metal.ConnectionMap) (*metal.Machine, error) {
	flatConnections := lo.Flatten(lo.Values(connections))
	connection, found := lo.Find(flatConnections, func(c metal.Connection) bool {
		return c.Nic.Name == nic.Name
	})

	if !found {
		return nil, nil
	}

	m, err := r.s.ds.Machine().Get(ctx, connection.MachineID)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func convertMachineConnections(machineConnections metal.ConnectionMap, nics []*apiv2.SwitchNic) ([]*apiv2.MachineConnection, error) {
	var (
		connections  = []*apiv2.MachineConnection{}
		notFoundNics = []string{}
	)

	for _, cons := range machineConnections {
		for _, con := range cons {
			nic, found := lo.Find(nics, func(n *apiv2.SwitchNic) bool {
				return n.Identifier == con.Nic.Identifier
			})

			if !found {
				notFoundNics = append(notFoundNics, con.Nic.Name)
				continue
			}
			connections = append(connections, &apiv2.MachineConnection{
				MachineId: con.MachineID,
				Nic:       nic,
			})
		}
	}

	if len(notFoundNics) > 0 {
		return nil, errorutil.InvalidArgument("nics %v could not be found but are connected to machines", notFoundNics)
	}

	return connections, nil
}

func updateNicNames(old, new metal.Nics) metal.Nics {
	var (
		updated metal.Nics
		oldNics = old.MapByIdentifier()
		newNics = new.MapByIdentifier()
	)

	for id, newNic := range newNics {
		oldNic, ok := oldNics[id]
		if !ok {
			updated = append(updated, *newNic)
			continue
		}

		updatedNic := *oldNic
		updatedNic.Name = newNic.Name
		updated = append(updated, updatedNic)
	}

	slices.SortStableFunc(updated, func(n1, n2 metal.Nic) int {
		return strings.Compare(n1.Identifier, n2.Identifier)
	})

	return updated
}

func makeBGPFilter(m *metal.Machine, projectMachines []*metal.Machine, vrf string, networks []*metal.Network, ips []*metal.IP) (*apiv2.BGPFilter, error) {
	if m == nil || m.Allocation == nil {
		return &apiv2.BGPFilter{}, nil
	}

	if m.Allocation.Role == metal.RoleFirewall {
		if vrf == "default" {
			return makeBGPFilterFirewall(m.Allocation.MachineNetworks)
		}
		return &apiv2.BGPFilter{}, nil
	}

	return makeBGPFilterMachine(m, projectMachines, metal.NetworksById(networks), metal.IPsByProject(ips))
}

func makeBGPFilterFirewall(machineNetworks []*metal.MachineNetwork) (*apiv2.BGPFilter, error) {
	vnis := []string{}
	cidrs := []string{}

	for _, net := range machineNetworks {
		if net == nil {
			continue
		}
		if net.Underlay {
			for _, ip := range net.IPs {
				ipwithMask, err := ipWithMask(ip)
				if err != nil {
					return nil, err
				}
				cidrs = append(cidrs, ipwithMask)
			}
		} else if net.Vrf != 0 {
			vnis = append(vnis, fmt.Sprintf("%d", net.Vrf))
			// filter for "project" addresses / cidrs is not possible since EVPN Type-5 routes can not be filtered by prefixes
		}
	}

	return &apiv2.BGPFilter{
		Cidrs: cidrs,
		Vnis:  vnis,
	}, nil
}

func makeBGPFilterMachine(m *metal.Machine, projectMachines []*metal.Machine, networks metal.NetworkMap, ips metal.IPsMap) (*apiv2.BGPFilter, error) {
	if m.Allocation == nil {
		return &apiv2.BGPFilter{}, nil
	}

	var (
		cidrs             []string
		private, underlay *metal.MachineNetwork
	)

	for _, net := range m.Allocation.MachineNetworks {
		if net == nil {
			continue
		}
		if net.Private {
			private = net
			continue
		}
		if net.Underlay {
			underlay = net
		}
	}

	if private != nil {
		cidrs = append(cidrs, private.Prefixes...)

		privateNetwork, ok := networks[private.NetworkID]
		if !ok {
			return nil, fmt.Errorf("no private network found for id:%s", private.NetworkID)
		}
		parentNetwork, ok := networks[privateNetwork.ParentNetworkID]
		if !ok {
			return nil, fmt.Errorf("parent network %s not found for id:%s", privateNetwork.ParentNetworkID, private.NetworkID)
		}
		if len(parentNetwork.AdditionalAnnouncableCIDRs) > 0 {
			cidrs = append(cidrs, parentNetwork.AdditionalAnnouncableCIDRs...)
		}
	}

	for _, i := range ips[m.Allocation.Project] {
		if underlay != nil && underlay.ContainsIP(i.IPAddress) {
			continue
		}

		ipAddress, err := i.GetIPAddress()
		if err != nil {
			return nil, err
		}
		if isFirewallIP(ipAddress, projectMachines) {
			continue
		}

		ipwithMask, err := ipWithMask(i.IPAddress)
		if err != nil {
			return nil, err
		}
		cidrs = append(cidrs, ipwithMask)
	}

	compactedCidrs, err := compactCidrs(cidrs)
	if err != nil {
		return nil, err
	}

	return &apiv2.BGPFilter{
		Cidrs: compactedCidrs,
		Vnis:  []string{},
	}, nil
}

func ipWithMask(ip string) (string, error) {
	parsed, err := netip.ParseAddr(ip)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%d", ip, parsed.BitLen()), nil
}

// compactCidrs finds the smallest sorted set of prefixes which covers all cidrs passed.
func compactCidrs(cidrs []string) ([]string, error) {
	var ipsetBuilder netipx.IPSetBuilder
	for _, cidr := range cidrs {
		parsed, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, err
		}
		ipsetBuilder.AddPrefix(parsed)
	}
	set, err := ipsetBuilder.IPSet()
	if err != nil {
		return nil, fmt.Errorf("unable to create ipset:%w", err)
	}
	var compactedCidrs []string
	for _, pfx := range set.Prefixes() {
		compactedCidrs = append(compactedCidrs, pfx.String())
	}
	return compactedCidrs, nil
}

func isFirewallIP(ip string, machines []*metal.Machine) bool {
	for _, m := range machines {
		if m.Allocation == nil || m.Allocation.Role != metal.RoleFirewall {
			continue
		}

		for _, nw := range m.Allocation.MachineNetworks {
			if nw == nil {
				continue
			}

			if slices.Contains(nw.IPs, ip) {
				return true
			}
		}
	}

	return false
}

func toMetalNics(switchNics []*apiv2.SwitchNic, hostname string) (metal.Nics, error) {
	var nics metal.Nics

	for _, switchNic := range switchNics {
		if switchNic == nil {
			continue
		}

		nic, err := toMetalNic(switchNic, hostname)
		if err != nil {
			return nil, err
		}

		nics = append(nics, *nic)
	}

	return nics, nil
}

func toMetalNic(switchNic *apiv2.SwitchNic, hostname string) (*metal.Nic, error) {
	bgpPortState, err := toSwitchBGPPortState(switchNic.BgpPortState)
	if err != nil {
		return nil, fmt.Errorf("failed to convert bgp port state: %w", err)
	}

	nicState, err := toSwitchPortState(switchNic.State)
	if err != nil {
		return nil, fmt.Errorf("failed to convert port state: %w", err)
	}

	return &metal.Nic{
		Name:         switchNic.Name,
		Hostname:     hostname,
		Identifier:   switchNic.Identifier,
		MacAddress:   switchNic.Mac,
		Vrf:          pointer.SafeDeref(switchNic.Vrf),
		State:        nicState,
		BGPPortState: bgpPortState,
	}, nil
}

func toMachineConnections(connections []*apiv2.MachineConnection, hostname string) (metal.ConnectionMap, error) {
	var (
		machineConnections = make(metal.ConnectionMap)
		connectedNics      []string
		duplicateNics      []string
	)

	for _, con := range connections {
		nic, err := toMetalNic(con.Nic, hostname)
		if err != nil {
			return nil, err
		}

		metalCons := machineConnections[con.MachineId]
		metalCons = append(metalCons, metal.Connection{
			Nic:       *nic,
			MachineID: con.MachineId,
		})

		machineConnections[con.MachineId] = metalCons

		if slices.Contains(connectedNics, nic.Identifier) {
			duplicateNics = append(duplicateNics, nic.Identifier)
		}
		connectedNics = append(connectedNics, nic.Identifier)
	}

	if len(duplicateNics) > 0 {
		return nil, errorutil.InvalidArgument("found multiple connections for nics %v", duplicateNics)
	}

	return machineConnections, nil
}

func toMetalSwitchSync(sync *apiv2.SwitchSync) *metal.SwitchSync {
	if sync == nil {
		return nil
	}

	var (
		t        time.Time
		duration time.Duration
	)

	if sync.Time != nil {
		t = sync.Time.AsTime()
	}
	if sync.Duration != nil {
		duration = sync.Duration.AsDuration()
	}

	return &metal.SwitchSync{
		Time:     t,
		Duration: duration,
		Error:    sync.Error,
	}
}

func adoptFromTwin(old, twin, new *metal.Switch) (*metal.Switch, error) {
	if new.Partition != old.Partition {
		return nil, errorutil.FailedPrecondition("old and new switch belong to different partitions, old: %v, new: %v", old.Partition, new.Partition)
	}
	if new.Rack != old.Rack {
		return nil, errorutil.FailedPrecondition("old and new switch belong to different racks, old: %v, new: %v", old.Rack, new.Rack)
	}
	if twin.ReplaceMode == metal.SwitchReplaceModeReplace {
		return nil, errorutil.FailedPrecondition("twin switch must not be in replace mode")
	}
	if len(twin.MachineConnections) == 0 {
		new.ReplaceMode = metal.SwitchReplaceModeOperational
		return new, nil
	}

	return adoptConfiguration(twin, new)
}

func adoptConfiguration(existing, new *metal.Switch) (*metal.Switch, error) {
	newNics, err := adoptNics(existing, new)
	if err != nil {
		return nil, fmt.Errorf("could not adopt existing nic configuration, err: %w", err)
	}

	newMachineConnections, err := adoptMachineConnections(existing, new)
	if err != nil {
		return nil, err
	}

	new.MachineConnections = newMachineConnections
	new.Nics = newNics
	new.ReplaceMode = metal.SwitchReplaceModeOperational

	return new, nil
}

func adoptNics(twin, newSwitch *metal.Switch) (metal.Nics, error) {
	var (
		newNics     metal.Nics
		missingNics []string
	)

	newNicMap, err := newSwitch.TranslateNicMap(twin.OS.Vendor)
	if err != nil {
		return nil, err
	}
	twinNicsByName := twin.Nics.MapByName()

	for name := range twinNicsByName {
		if _, ok := newNicMap[name]; !ok {
			missingNics = append(missingNics, name)
		}
	}

	if len(missingNics) > 0 {
		return nil, errorutil.FailedPrecondition("new switch misses the nics %v - check the breakout configuration of the switch ports of switch %s", missingNics, newSwitch.Name)
	}

	for name, nic := range newNicMap {
		if twinNic, ok := twinNicsByName[name]; ok {
			nic.Vrf = twinNic.Vrf
		}
		newNics = append(newNics, *nic)
	}

	sort.SliceStable(newNics, func(i, j int) bool {
		return newNics[i].Name < newNics[j].Name
	})

	return newNics, nil
}

func adoptMachineConnections(twin, newSwitch *metal.Switch) (metal.ConnectionMap, error) {
	var (
		newConnectionMap = metal.ConnectionMap{}
		missingNics      []string
	)

	newNicMap, err := newSwitch.TranslateNicMap(twin.OS.Vendor)
	if err != nil {
		return nil, err
	}

	for mid, cons := range twin.MachineConnections {
		var newConnections metal.Connections

		for _, con := range cons {
			if n, ok := newNicMap[con.Nic.Name]; ok {
				newCon := con
				newCon.Nic.Name = n.Name
				newCon.Nic.Identifier = n.Identifier
				newCon.Nic.MacAddress = n.MacAddress
				newConnections = append(newConnections, newCon)
			} else {
				missingNics = append(missingNics, con.Nic.Name)
			}
		}

		newConnectionMap[mid] = newConnections
	}

	if len(missingNics) > 0 {
		return nil, errorutil.Internal("twin switch %s has machine connections with switch ports %v which are missing on the new switch %s", twin.ID, missingNics, newSwitch.ID)
	}

	return newConnectionMap, nil
}

func (r *switchRepository) migrateMachineConnections(ctx context.Context, old, new *metal.Switch) error {
	nicMap, err := new.TranslateNicMap(old.OS.Vendor)
	if err != nil {
		return err
	}

	err = r.adjustMachineConnections(ctx, old.MachineConnections, nicMap)
	if err != nil {
		return err
	}

	old.MachineConnections = nil
	return r.s.ds.Switch().Update(ctx, old)
}

func (r *switchRepository) adjustMachineConnections(ctx context.Context, oldConnections metal.ConnectionMap, nicMap metal.NicMap) error {
	for mid, cons := range oldConnections {
		m, err := r.s.ds.Machine().Get(ctx, mid)
		if err != nil {
			return err
		}

		newNics, err := adjustMachineNics(m.Hardware.Nics, cons, nicMap)
		if err != nil {
			return err
		}

		m.Hardware.Nics = newNics
		err = r.s.ds.Machine().Update(ctx, m)
		if err != nil {
			return err
		}
	}

	return nil
}

func adjustMachineNics(nics metal.Nics, connections metal.Connections, nicMap metal.NicMap) (metal.Nics, error) {
	var newNics metal.Nics

	for _, nic := range nics {
		var newNeighbors metal.Nics

		for _, neigh := range nic.Neighbors {
			if nicInConnections(neigh.Name, neigh.MacAddress, connections) {
				n, ok := nicMap[neigh.Name]
				if !ok {
					return nil, fmt.Errorf("unable to find corresponding new neighbor nic for %s", neigh.Name)
				}
				newNeighbors = append(newNeighbors, *n)
			} else {
				newNeighbors = append(newNeighbors, neigh)
			}
		}

		nic.Neighbors = newNeighbors
		newNics = append(newNics, nic)
	}

	return newNics, nil
}

func nicInConnections(name string, mac string, connections metal.Connections) bool {
	for _, con := range connections {
		if con.Nic.Name == name && con.Nic.MacAddress == mac {
			return true
		}
	}
	return false
}
