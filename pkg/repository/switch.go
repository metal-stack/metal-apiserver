package repository

import (
	"context"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/types/known/durationpb"
)

type switchRepository struct {
	s *Store
}

func (r *switchRepository) get(ctx context.Context, id string) (*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) validateCreate(ctx context.Context, req *infrav2.SwitchServiceCreateRequest) error {
	panic("unimplemented")
}

func (r *switchRepository) create(ctx context.Context, req *infrav2.SwitchServiceCreateRequest) (*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) validateUpdate(ctx context.Context, req *adminv2.SwitchServiceUpdateRequest, oldSwitch *metal.Switch) error {
	panic("unimplemented")
}

func (r *switchRepository) update(ctx context.Context, oldSwitch *metal.Switch, req *adminv2.SwitchServiceUpdateRequest) (*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) validateDelete(ctx context.Context, sw *metal.Switch) error {
	panic("unimplemented")
}

func (r *switchRepository) delete(ctx context.Context, sw *metal.Switch) error {
	panic("unimplemented")
}

func (r *switchRepository) find(ctx context.Context, query *apiv2.SwitchQuery) (*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) list(ctx context.Context, query *apiv2.SwitchQuery) ([]*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) convertToInternal(sw *apiv2.Switch) (*metal.Switch, error) {
	if sw == nil {
		return nil, nil
	}

	var nics metal.Nics
	for _, nic := range sw.Nics {
		var bgpPortState *metal.BGPPortState
		if nic.BgpPortState != nil {
			bgpState, err := metal.ToBGPState(nic.BgpPortState.BgpState)
			if err != nil {
				return nil, err
			}

			bgpPortState = &metal.BGPPortState{
				Neighbor:              nic.BgpPortState.Neighbor,
				PeerGroup:             nic.BgpPortState.PeerGroup,
				VrfName:               nic.BgpPortState.VrfName,
				State:                 bgpState,
				BGPTimerUpEstablished: nic.BgpPortState.BgpTimerUpEstablished.AsDuration(),
				SentPrefixCounter:     nic.BgpPortState.SentPrefixCounter,
				AcceptedPrefixCounter: nic.BgpPortState.AcceptedPrefixCounter,
			}
		}

		desiredState, err := metal.ToSwitchPortStatus(nic.State.Desired)
		if err != nil {
			return nil, err
		}
		actualState, err := metal.ToSwitchPortStatus(nic.State.Actual)
		if err != nil {
			return nil, err
		}

		nics = append(nics, metal.Nic{
			Name:       nic.Name,
			Identifier: nic.Identifier,
			Mac:        nic.Mac,
			Vrf:        nic.Vrf,
			State: &metal.NicState{
				Desired: desiredState,
				Actual:  actualState,
			},
			BGPPortState: bgpPortState,
		})
	}

	replaceMode, err := metal.ToReplaceMode(sw.ReplaceMode)
	if err != nil {
		return nil, err
	}
	vendor, err := metal.ToSwitchOSVendor(sw.Os.Vendor)
	if err != nil {
		return nil, err
	}

	return &metal.Switch{
		Base: metal.Base{
			ID:          sw.Id,
			Name:        sw.Id,
			Description: sw.Description,
		},
		RackID:         pointer.SafeDeref(sw.Rack),
		Partition:      sw.Partition,
		ReplaceMode:    replaceMode,
		ManagementIP:   sw.ManagementIp,
		ManagementUser: sw.ManagementUser,
		ConsoleCommand: sw.ConsoleCommand,
		// FIXME populate machine connections
		MachineConnections: make(metal.ConnectionMap),
		OS: metal.SwitchOS{
			Vendor:           vendor,
			Version:          sw.Os.Version,
			MetalCoreVersion: sw.Os.MetalCoreVersion,
		},
		Nics: nics,
	}, nil
}

func (r *switchRepository) convertToProto(sw *metal.Switch) (*apiv2.Switch, error) {
	if sw == nil {
		return nil, nil
	}

	var nics []*apiv2.SwitchNic
	for _, nic := range sw.Nics {
		var bgpPortState *apiv2.SwitchBGPPortState
		if nic.BGPPortState != nil {
			bgpState, err := metal.FromBGPState(nic.BGPPortState.State)
			if err != nil {
				return nil, err
			}

			bgpPortState = &apiv2.SwitchBGPPortState{
				Neighbor:              nic.BGPPortState.Neighbor,
				PeerGroup:             nic.BGPPortState.PeerGroup,
				VrfName:               nic.BGPPortState.VrfName,
				BgpState:              bgpState,
				BgpTimerUpEstablished: durationpb.New(nic.BGPPortState.BGPTimerUpEstablished),
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

		nics = append(nics, &apiv2.SwitchNic{
			Name:       nic.Name,
			Identifier: nic.Identifier,
			Mac:        nic.Mac,
			Vrf:        new(string),
			State: &apiv2.NicState{
				Desired: desiredState,
				Actual:  actualState,
			},
			// FIXME need machine connections to make bgp filters
			BgpFilter:    &apiv2.BGPFilter{},
			BgpPortState: bgpPortState,
		})
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
		Id:             sw.ID,
		Description:    sw.Description,
		Rack:           &sw.RackID,
		Partition:      sw.Partition,
		ReplaceMode:    replaceMode,
		ManagementIp:   sw.ManagementIP,
		ManagementUser: sw.ManagementUser,
		ConsoleCommand: sw.ConsoleCommand,
		Nics:           nics,
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

func (r *switchRepository) Migrate(ctx context.Context, oldSwitch, newSwitch string) (*metal.Switch, error) {
	panic("unimplemented")
}

func (r *switchRepository) Port(ctx context.Context, id, port string, status apiv2.SwitchPortStatus) (*metal.Switch, error) {
	panic("unimplemented")
}
