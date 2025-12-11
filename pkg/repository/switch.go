package repository

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
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
		LastSync      *infrav2.SwitchSync
		LastSyncError *infrav2.SwitchSync
	}
)

func (s *SwitchStatus) GetID() string {
	return s.ID
}

func (r *switchRepository) Register(ctx context.Context, req *infrav2.SwitchServiceRegisterRequest) (*apiv2.Switch, error) {
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
		err = r.validateReplace(ctx, old, new)
		if err != nil {
			return nil, err
		}

		return r.replace(ctx, old, new)
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
	panic("unimplemented")
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

func (r *switchRepository) ConnectMachineWithSwitches(m *apiv2.Machine) error {
	panic("unimplemented")
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
		status.LastSync = &infrav2.SwitchSync{
			Time:     timestamppb.New(metalStatus.LastSync.Time),
			Duration: durationpb.New(metalStatus.LastSync.Duration),
			Error:    metalStatus.LastSync.Error,
		}
	}

	if metalStatus.LastSyncError != nil {
		status.LastSyncError = &infrav2.SwitchSync{
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
	updated, err := updateAllButNics(sw, req)
	if err != nil {
		return nil, err
	}

	if len(req.Nics) > 0 {
		nics, err := toMetalNics(req.Nics)
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

func (r *switchRepository) replace(ctx context.Context, oldSwitch, newSwitch *apiv2.Switch) (*apiv2.Switch, error) {
	panic("unimplemented")
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

	nics, err := toMetalNics(sw.Nics)
	if err != nil {
		return nil, err
	}

	replaceMode, err := metal.ToReplaceMode(sw.ReplaceMode)
	if err != nil {
		return nil, err
	}
	vendor, err := metal.ToSwitchOSVendor(sw.Os.Vendor)
	if err != nil {
		return nil, err
	}

	connections, err := toMachineConnections(sw.MachineConnections)
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

	nics, err := r.toSwitchNics(ctx, sw)
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

func (r *switchRepository) updateOnRegister(ctx context.Context, sw *metal.Switch, req *adminv2.SwitchServiceUpdateRequest) (*metal.Switch, error) {
	updated, err := updateAllButNics(sw, req)
	if err != nil {
		return nil, err
	}

	if len(req.Nics) > 0 {
		nics, err := toMetalNics(req.Nics)
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

func (r *switchRepository) toSwitchNics(ctx context.Context, sw *metal.Switch) ([]*apiv2.SwitchNic, error) {
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
		connections  []*apiv2.MachineConnection
		notFoundNics []string
	)

	for _, cons := range machineConnections {
		for _, con := range cons {
			nic, found := lo.Find(nics, func(n *apiv2.SwitchNic) bool {
				return n.Identifier == con.Nic.Identifier
			})

			if !found {
				notFoundNics = append(notFoundNics, con.Nic.Identifier)
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

func toMetalNics(switchNics []*apiv2.SwitchNic) (metal.Nics, error) {
	var nics metal.Nics

	for _, switchNic := range switchNics {
		if switchNic == nil {
			continue
		}

		nic, err := toMetalNic(switchNic)
		if err != nil {
			return nil, err
		}

		nics = append(nics, *nic)
	}

	return nics, nil
}

func toMetalNic(switchNic *apiv2.SwitchNic) (*metal.Nic, error) {
	bgpPortState, err := toSwitchBGPPortState(switchNic.BgpPortState)
	if err != nil {
		return nil, fmt.Errorf("failed to convert bgp port state: %w", err)
	}

	nicState, err := toSwitchPortState(switchNic.State)
	if err != nil {
		return nil, fmt.Errorf("failed to convert port state: %w", err)
	}

	return &metal.Nic{
		// TODO: what about hostname and neighbors?
		Name:         switchNic.Name,
		Identifier:   switchNic.Identifier,
		MacAddress:   switchNic.Mac,
		Vrf:          pointer.SafeDeref(switchNic.Vrf),
		State:        nicState,
		BGPPortState: bgpPortState,
	}, nil
}

func toMachineConnections(connections []*apiv2.MachineConnection) (metal.ConnectionMap, error) {
	var (
		machineConnections = make(metal.ConnectionMap)
		connectedNics      []string
		duplicateNics      []string
	)

	for _, con := range connections {
		nic, err := toMetalNic(con.Nic)
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

func toMetalSwitchSync(sync *infrav2.SwitchSync) *metal.SwitchSync {
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
