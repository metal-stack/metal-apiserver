package repository

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

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
	"google.golang.org/protobuf/types/known/durationpb"
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

	if sw.ReplaceMode == metal.ReplaceModeReplace {
		err = r.validateReplace(ctx, old, new)
		if err != nil {
			return nil, err
		}
		return r.replace(ctx, old, new)
	}

	updateReq := &adminv2.SwitchServiceUpdateRequest{
		Id:             new.Id,
		Description:    pointer.PointerOrNil(new.Description),
		RackId:         new.Rack,
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
	if req.RackId != nil {
		new.RackID = *req.RackId
	}
	if req.ReplaceMode != nil {
		replaceMode, err := metal.ToReplaceMode(*req.ReplaceMode)
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
		nics, err := metal.ToMetalNics(req.Nics)
		if err != nil {
			return nil, err
		}
		new.Nics = updateNics(oldSwitch.Nics, nics)
	}
	if req.Os != nil {
		vendor, err := metal.ToSwitchOSVendor(req.Os.Vendor)
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

	nics, err := metal.ToMetalNics(sw.Nics)
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

	connections, err := metal.ToMachineConnections(sw.MachineConnections)
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

	nics, err := r.toSwitchNics(ctx, sw.Nics, sw.MachineConnections)
	if err != nil {
		return nil, err
	}

	connections := convertMachineConnections(sw.MachineConnections, nics)

	replaceMode, err := metal.FromReplaceMode(sw.ReplaceMode)
	if err != nil {
		return nil, err
	}
	vendor, err := metal.FromSwitchOSVendor(sw.OS.Vendor)
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

func (r *switchRepository) toSwitchNics(ctx context.Context, nics metal.Nics, connections metal.ConnectionMap) ([]*apiv2.SwitchNic, error) {
	networks, err := r.s.ds.Network().List(ctx)
	if err != nil {
		return nil, err
	}

	ips, err := r.s.ds.IP().List(ctx)
	if err != nil {
		return nil, err
	}

	var switchNics []*apiv2.SwitchNic
	for _, nic := range nics {
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
				BgpTimerUpEstablished: durationpb.New(time.Duration(nic.BGPPortState.BgpTimerUpEstablished)),
				SentPrefixCounter:     nic.BGPPortState.SentPrefixCounter,
				AcceptedPrefixCounter: nic.BGPPortState.AcceptedPrefixCounter,
			}
		}

		desiredState, err := metal.FromSwitchPortStatus(nic.State.Desired)
		if err != nil {
			return nil, err
		}
		actualState, err := metal.FromSwitchPortStatus(nic.State.Actual)
		if err != nil {
			return nil, err
		}

		m, err := r.getConnectedMachineForNic(ctx, nic, connections)
		if err != nil {
			return nil, err
		}

		var projectMachines []*metal.Machine
		if m != nil && m.Allocation != nil {
			projectMachines, err = r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{
				Allocation: &apiv2.MachineAllocationQuery{
					Project: pointer.Pointer(m.Allocation.Project),
				},
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

func convertMachineConnections(machineConnections metal.ConnectionMap, nics []*apiv2.SwitchNic) []*apiv2.MachineConnection {
	var connections []*apiv2.MachineConnection

	for mid, cons := range machineConnections {
		nic, found := lo.Find(nics, func(n *apiv2.SwitchNic) bool {
			if len(cons) < 1 {
				return false
			}
			// TODO: is it okay to only add one connection here or should api switch also allow multiple connections for a machine?
			return n.Name == cons[0].Nic.Name
		})
		if found {
			connections = append(connections, &apiv2.MachineConnection{
				MachineId: mid,
				Nic:       nic,
			})
		}
	}

	return connections
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
	parsedIP, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}

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

			if lo.ContainsBy(nw.Prefixes, func(p string) bool {
				pfx, err := netip.ParsePrefix(p)
				if err != nil {
					return false
				}

				return pfx.Contains(parsedIP)
			}) {
				return true
			}
		}
	}

	return false
}
