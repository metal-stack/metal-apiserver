package repository

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/metal-stack/api/go/enum"
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
	"google.golang.org/protobuf/types/known/timestamppb"
)

type switchRepository struct {
	s *Store
}

type SwitchServiceCreateRequest struct {
	Switch *apiv2.Switch
}

func (r *switchRepository) Register(ctx context.Context, req *infrav2.SwitchServiceRegisterRequest) (*metal.Switch, error) {
	sw, err := r.s.ds.Switch().Get(ctx, req.Switch.Id)
	if err != nil && !errorutil.IsNotFound(err) {
		return nil, err
	}
	if errorutil.IsNotFound(err) {
		rq := &SwitchServiceCreateRequest{Switch: req.Switch}
		err = r.validateCreate(ctx, rq)
		if err != nil {
			return nil, err
		}
		return r.create(ctx, rq)
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
		Id:             new.Id,
		Description:    pointer.PointerOrNil(new.Description),
		ReplaceMode:    pointer.PointerOrNil(new.ReplaceMode),
		ManagementIp:   pointer.PointerOrNil(new.ManagementIp),
		ManagementUser: pointer.PointerOrNil(new.ManagementUser),
		ConsoleCommand: pointer.PointerOrNil(new.ConsoleCommand),
		Nics:           new.Nics,
		Os:             new.Os,
	}

	err = r.validateUpdate(ctx, updateReq, sw)
	if err != nil {
		return nil, err
	}

	return r.update(ctx, sw, updateReq)
}

func (r *switchRepository) Migrate(ctx context.Context, oldSwitch, newSwitch string) (*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) Port(ctx context.Context, id, port string, status apiv2.SwitchPortStatus) (*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) ConnectMachineWithSwitches(m *metal.Machine) error {
	panic("unimplemented")
}

func (r *switchRepository) get(ctx context.Context, id string) (*metal.Switch, error) {
	sw, err := r.s.ds.Switch().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return sw, nil
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

func (r *switchRepository) update(ctx context.Context, oldSwitch *metal.Switch, req *adminv2.SwitchServiceUpdateRequest) (*metal.Switch, error) {
	new := *oldSwitch

	if req.Description != nil {
		new.Description = *req.Description
	}
	if req.ReplaceMode != nil {
		replaceMode, err := toReplaceMode(*req.ReplaceMode)
		if err != nil {
			return nil, err
		}
		new.ReplaceMode = replaceMode
	}
	if req.ManagementIp != nil {
		new.ManagementIP = *req.ManagementIp
	}
	if req.ManagementUser != nil {
		new.ManagementUser = *req.ManagementUser
	}
	if req.ConsoleCommand != nil {
		new.ConsoleCommand = *req.ConsoleCommand
	}
	if len(req.Nics) > 0 {
		nics, err := toMetalNics(req.Nics)
		if err != nil {
			return nil, err
		}
		new.Nics = updateNics(oldSwitch.Nics, nics)
	}
	if req.Os != nil {
		vendor, err := toSwitchOSVendor(req.Os.Vendor)
		if err != nil {
			return nil, err
		}
		new.OS = metal.SwitchOS{
			Vendor:           vendor,
			Version:          req.Os.Version,
			MetalCoreVersion: req.Os.MetalCoreVersion,
		}
	}

	err := r.s.ds.Switch().Update(ctx, &new)
	if err != nil {
		return nil, err
	}

	return &new, nil
}

func (r *switchRepository) delete(ctx context.Context, sw *metal.Switch) error {
	// FIX: also delete switch status
	return r.s.ds.Switch().Delete(ctx, sw)
}

func (r *switchRepository) replace(ctx context.Context, oldSwitch, newSwitch *apiv2.Switch) (*metal.Switch, error) {
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

	replaceMode, err := toReplaceMode(sw.ReplaceMode)
	if err != nil {
		return nil, err
	}
	vendor, err := toSwitchOSVendor(sw.Os.Vendor)
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
		RackID:             pointer.SafeDeref(sw.Rack),
		Partition:          sw.Partition,
		ReplaceMode:        replaceMode,
		ManagementIP:       sw.ManagementIp,
		ManagementUser:     sw.ManagementUser,
		ConsoleCommand:     sw.ConsoleCommand,
		MachineConnections: connections,
		OS: metal.SwitchOS{
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

	replaceMode, err := fromReplaceMode(sw.ReplaceMode)
	if err != nil {
		return nil, err
	}
	vendor, err := fromSwitchOSVendor(sw.OS.Vendor)
	if err != nil {
		return nil, err
	}

	return &apiv2.Switch{
		Id:                 sw.ID,
		Description:        sw.Description,
		Rack:               pointer.PointerOrNil(sw.RackID),
		Partition:          sw.Partition,
		ReplaceMode:        replaceMode,
		ManagementIp:       sw.ManagementIP,
		ManagementUser:     sw.ManagementUser,
		ConsoleCommand:     sw.ConsoleCommand,
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

func (r *switchRepository) toSwitchNics(ctx context.Context, sw *metal.Switch) ([]*apiv2.SwitchNic, error) {
	networks, err := r.s.ds.Network().List(ctx)
	if err != nil {
		return nil, err
	}

	ips, err := r.s.ds.IP().List(ctx)
	if err != nil {
		return nil, err
	}

	var switchNics []*apiv2.SwitchNic
	for _, nic := range sw.Nics {
		var bgpPortState *apiv2.SwitchBGPPortState
		if nic.BGPPortState != nil {
			bgpState, err := fromBGPState(nic.BGPPortState.BgpState)
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

		desiredState, err := fromSwitchPortStatus(nic.State.Desired)
		if err != nil {
			return nil, err
		}
		actualState, err := fromSwitchPortStatus(&nic.State.Actual)
		if err != nil {
			return nil, err
		}

		m, err := r.getConnectedMachineForNic(ctx, nic, sw.MachineConnections)
		if err != nil {
			return nil, err
		}

		var projectMachines []*metal.Machine
		if m != nil && m.Allocation != nil {
			projectMachines, err = r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{
				Allocation: &apiv2.MachineAllocationQuery{
					Project: pointer.Pointer(m.Allocation.Project),
				},
				Partition: pointer.Pointer(sw.Partition),
				Rack:      pointer.Pointer(sw.RackID),
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
				Desired: desiredState,
				Actual:  actualState,
			},
			BgpFilter:    filter,
			BgpPortState: bgpPortState,
		})
	}

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

func updateNics(old, new metal.Nics) metal.Nics {
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
	var (
		bgpPortState *metal.SwitchBGPPortState
		nicState     *metal.NicState
	)

	if switchNic.BgpPortState != nil {
		bgpState, err := toBGPState(switchNic.BgpPortState.BgpState)
		if err != nil {
			return nil, err
		}

		bgpPortState = &metal.SwitchBGPPortState{
			Neighbor:              switchNic.BgpPortState.Neighbor,
			PeerGroup:             switchNic.BgpPortState.PeerGroup,
			VrfName:               switchNic.BgpPortState.VrfName,
			BgpState:              bgpState,
			BgpTimerUpEstablished: uint64(switchNic.BgpPortState.BgpTimerUpEstablished.Seconds),
			SentPrefixCounter:     switchNic.BgpPortState.SentPrefixCounter,
			AcceptedPrefixCounter: switchNic.BgpPortState.AcceptedPrefixCounter,
		}
	}

	if switchNic.State != nil {
		desiredState, err := toSwitchPortStatus(switchNic.State.Desired)
		if err != nil {
			return nil, err
		}
		actualState, err := toSwitchPortStatus(switchNic.State.Actual)
		if err != nil {
			return nil, err
		}
		nicState = &metal.NicState{
			Desired: &desiredState,
			Actual:  actualState,
		}
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

func toReplaceMode(mode apiv2.SwitchReplaceMode) (metal.SwitchReplaceMode, error) {
	strVal, err := enum.GetStringValue(mode)
	if err != nil {
		return metal.SwitchReplaceMode(""), err
	}
	return metal.SwitchReplaceMode(*strVal), nil
}

func fromReplaceMode(mode metal.SwitchReplaceMode) (apiv2.SwitchReplaceMode, error) {
	apiv2ReplaceMode, err := enum.GetEnum[apiv2.SwitchReplaceMode](string(mode))
	if err != nil {
		return apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_UNSPECIFIED, fmt.Errorf("switch replace mode:%q is invalid", mode)
	}
	return apiv2ReplaceMode, nil
}

func toSwitchOSVendor(vendor apiv2.SwitchOSVendor) (metal.SwitchOSVendor, error) {
	strVal, err := enum.GetStringValue(vendor)
	if err != nil {
		return metal.SwitchOSVendor(""), err
	}
	return metal.SwitchOSVendor(*strVal), nil
}

func fromSwitchOSVendor(vendor metal.SwitchOSVendor) (apiv2.SwitchOSVendor, error) {
	apiv2Vendor, err := enum.GetEnum[apiv2.SwitchOSVendor](string(vendor))
	if err != nil {
		return apiv2.SwitchOSVendor_SWITCH_OS_VENDOR_UNSPECIFIED, fmt.Errorf("switch os vendor: %q is invalid", vendor)
	}
	return apiv2Vendor, nil
}

func toSwitchPortStatus(status apiv2.SwitchPortStatus) (metal.SwitchPortStatus, error) {
	strVal, err := enum.GetStringValue(status)
	if err != nil {
		return metal.SwitchPortStatus(""), err
	}
	return metal.SwitchPortStatus(strings.ToUpper(*strVal)), nil
}

func fromSwitchPortStatus(status *metal.SwitchPortStatus) (apiv2.SwitchPortStatus, error) {
	if status == nil {
		return apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNSPECIFIED, nil
	}

	apiv2Status, err := enum.GetEnum[apiv2.SwitchPortStatus](strings.ToLower(string(*status)))
	if err != nil {
		return apiv2.SwitchPortStatus_SWITCH_PORT_STATUS_UNSPECIFIED, fmt.Errorf("switch port status: %q is invalid", *status)
	}
	return apiv2Status, nil
}

func toBGPState(state apiv2.BGPState) (metal.BGPState, error) {
	strVal, err := enum.GetStringValue(state)
	if err != nil {
		return metal.BGPState(""), err
	}
	return metal.BGPState(*strVal), nil
}

func fromBGPState(state metal.BGPState) (apiv2.BGPState, error) {
	apiv2State, err := enum.GetEnum[apiv2.BGPState](string(state))
	if err != nil {
		return apiv2.BGPState_BGP_STATE_UNSPECIFIED, fmt.Errorf("bgp state: %q is invalid", state)
	}
	return apiv2State, nil
}
