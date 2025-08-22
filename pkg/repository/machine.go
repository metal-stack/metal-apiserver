package repository

import (
	"context"
	"slices"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/metal-stack/metal-lib/pkg/tag"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	machineRepository struct {
		s     *Store
		scope *ProjectScope
	}
)

func (r *machineRepository) get(ctx context.Context, id string) (*metal.Machine, error) {
	machine, err := r.s.ds.Machine().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return machine, nil
}

func (r *machineRepository) matchScope(machine *metal.Machine) bool {
	if r.scope == nil {
		return true
	}

	if machine.Allocation == nil {
		return true
	}

	return r.scope.projectID == pointer.SafeDeref(machine).Allocation.Project
}

func (r *machineRepository) create(ctx context.Context, req *apiv2.MachineServiceCreateRequest) (*metal.Machine, error) {
	panic("unimplemented")
}

func (r *machineRepository) update(ctx context.Context, m *metal.Machine, req *apiv2.MachineServiceUpdateRequest) (*metal.Machine, error) {
	if m.Allocation == nil {
		return m, errorutil.InvalidArgument("only allocated machines can be updated")
	}

	if req.Description != nil {
		m.Allocation.Description = *req.Description
	}

	if req.Labels != nil {
		m.Allocation.Labels = updateLabelsOnMap(req.Labels, m.Allocation.Labels)
	}

	if len(req.SshPublicKeys) > 0 && m.Allocation != nil {
		m.Allocation.SSHPubKeys = req.SshPublicKeys
	}
	if err := r.s.ds.Machine().Update(ctx, m); err != nil {
		return nil, errorutil.Convert(err)
	}

	return m, nil
}

func (r *machineRepository) delete(ctx context.Context, m *metal.Machine) error {
	var (
		uuid           *string
		allocationUUID *string
	)
	uuid = &m.ID
	if m.Allocation != nil {
		uuid = nil
		allocationUUID = &m.Allocation.UUID
	}
	info, err := r.s.async.NewMachineDeleteTask(uuid, allocationUUID)
	if err != nil {
		return err
	}

	r.s.log.Info("machine delete queued", "info", info)

	return nil
}

func (r *machineRepository) find(ctx context.Context, rq *apiv2.MachineQuery) (*metal.Machine, error) {
	ip, err := r.s.ds.Machine().Find(ctx, r.scopedMachineFilters(queries.MachineFilter(rq))...)
	if err != nil {
		return nil, err
	}

	return ip, nil
}

func (r *machineRepository) list(ctx context.Context, rq *apiv2.MachineQuery) ([]*metal.Machine, error) {
	machines, err := r.s.ds.Machine().List(ctx, r.scopedMachineFilters(queries.MachineFilter(rq))...)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(machines, func(a, b *metal.Machine) int {
		return strings.Compare(a.ID, b.ID)
	})

	return machines, nil
}

func (r *machineRepository) convertToInternal(machine *apiv2.Machine) (*metal.Machine, error) {
	panic("unimplemented")
}

func (r *machineRepository) convertToProto(m *metal.Machine) (*apiv2.Machine, error) {
	var (
		ctx              = context.Background()
		labels           *apiv2.Labels
		allocationLabels *apiv2.Labels
		bios             *apiv2.MachineBios
		allocation       *apiv2.MachineAllocation
		condition        *apiv2.MachineCondition
		status           *apiv2.MachineStatus
		size             *apiv2.Size
		vpn              *apiv2.MachineVPN
		dnsServers       []*apiv2.DNSServer
		ntpServers       []*apiv2.NTPServer
		firewallRules    *apiv2.FirewallRules
		machineNetworks  []*apiv2.MachineNetwork
		filesystemLayout *apiv2.FilesystemLayout
	)

	if len(m.Tags) > 0 {
		labels = &apiv2.Labels{
			Labels: tag.NewTagMap(m.Tags),
		}
	}
	// Fetch Partition
	partition, err := r.s.ds.Partition().Get(ctx, m.PartitionID)
	if err != nil {
		return nil, err
	}
	apiv2Partition, err := r.s.Partition().ConvertToProto(partition)
	if err != nil {
		return nil, err
	}
	metalSize, err := r.s.Size().Get(ctx, m.SizeID)
	if err != nil {
		return nil, err
	}
	size, err = r.s.Size().ConvertToProto(metalSize)
	if err != nil {
		return nil, err
	}

	var (
		disks []*apiv2.MachineBlockDevice
		cpus  []*apiv2.MetalCPU
		gpus  []*apiv2.MetalGPU
		nics  []*apiv2.MachineNic
	)

	for _, disk := range m.Hardware.Disks {
		disks = append(disks, &apiv2.MachineBlockDevice{
			Name: disk.Name,
			Size: disk.Size,
		})
	}

	for _, cpu := range m.Hardware.MetalCPUs {
		cpus = append(cpus, &apiv2.MetalCPU{
			Vendor:  cpu.Vendor,
			Model:   cpu.Model,
			Cores:   cpu.Cores,
			Threads: cpu.Threads,
		})
	}

	for _, gpu := range m.Hardware.MetalGPUs {
		gpus = append(gpus, &apiv2.MetalGPU{
			Vendor: gpu.Model,
			Model:  gpu.Model,
		})
	}

	for _, nic := range m.Hardware.Nics {
		var neighs []*apiv2.MachineNic
		for _, neigh := range nic.Neighbors {
			neighs = append(neighs, &apiv2.MachineNic{
				Mac:        string(neigh.MacAddress),
				Name:       neigh.Name,
				Identifier: neigh.Identifier,
			})
		}
		nics = append(nics, &apiv2.MachineNic{
			Mac:        string(nic.MacAddress),
			Name:       nic.Name,
			Identifier: nic.Identifier,
			Neighbors:  neighs,
			// TODO need we more in apiv2.MachineNic
		})
	}

	hardware := &apiv2.MachineHardware{
		Memory: m.Hardware.Memory,
		Disks:  disks,
		Cpus:   cpus,
		Gpus:   gpus,
		Nics:   nics,
	}

	bios = &apiv2.MachineBios{
		Version: m.BIOS.Version,
		Vendor:  m.BIOS.Vendor,
		Date:    m.BIOS.Date,
	}

	if m.Allocation != nil {
		alloc := m.Allocation
		image, err := r.s.ds.Image().Get(ctx, alloc.ImageID)
		if err != nil {
			return nil, err
		}
		apiv2Image, err := r.s.Image().ConvertToProto(image)
		if err != nil {
			return nil, err
		}

		if alloc.FilesystemLayout != nil {
			fsl, err := r.s.ds.FilesystemLayout().Get(ctx, alloc.FilesystemLayout.ID)
			if err != nil {
				return nil, err
			}
			filesystemLayout, err = r.s.FilesystemLayout().ConvertToProto(fsl)
			if err != nil {
				return nil, err
			}
		}

		if alloc.VPN != nil {
			vpn = &apiv2.MachineVPN{
				ControlPlaneAddress: alloc.VPN.ControlPlaneAddress,
				AuthKey:             alloc.VPN.AuthKey,
				Connected:           alloc.VPN.Connected,
			}
		}
		for _, dns := range alloc.DNSServers {
			dnsServers = append(dnsServers, &apiv2.DNSServer{
				Ip: dns.IP,
			})
		}
		for _, ntp := range alloc.NTPServers {
			ntpServers = append(ntpServers, &apiv2.NTPServer{
				Address: ntp.Address,
			})
		}
		if alloc.FirewallRules != nil {
			var (
				egress  []*apiv2.FirewallEgressRule
				ingress []*apiv2.FirewallIngressRule
			)
			for _, e := range alloc.FirewallRules.Egress {
				protocol, err := enum.GetEnum[apiv2.IPProtocol](strings.ToLower(string(e.Protocol)))
				if err != nil {
					return nil, err
				}
				var ports []uint32
				for _, p := range e.Ports {
					ports = append(ports, uint32(p))
				}
				egress = append(egress, &apiv2.FirewallEgressRule{
					Protocol: protocol,
					Ports:    ports,
					To:       e.To,
					Comment:  e.Comment,
				})
			}
			for _, i := range alloc.FirewallRules.Ingress {
				protocol, err := enum.GetEnum[apiv2.IPProtocol](strings.ToLower(string(i.Protocol)))
				if err != nil {
					return nil, err
				}
				var ports []uint32
				for _, p := range i.Ports {
					ports = append(ports, uint32(p))
				}
				ingress = append(ingress, &apiv2.FirewallIngressRule{
					Protocol: protocol,
					Ports:    ports,
					To:       i.To,
					From:     i.From,
					Comment:  i.Comment,
				})
			}
			firewallRules = &apiv2.FirewallRules{
				Egress:  egress,
				Ingress: ingress,
			}
		}

		for _, nw := range alloc.MachineNetworks {
			metalNetwork, err := r.s.UnscopedNetwork().Get(ctx, nw.NetworkID)
			if err != nil {
				return nil, err
			}
			networkType, err := metal.FromNetworkType(*metalNetwork.NetworkType)
			if err != nil {
				return nil, err
			}
			natType, err := metal.FromNATType(*metalNetwork.NATType)
			if err != nil {
				return nil, err
			}

			machineNetworks = append(machineNetworks, &apiv2.MachineNetwork{
				Network:             nw.NetworkID,
				Ips:                 nw.IPs,
				Prefixes:            nw.Prefixes,            // TODO would be better if we fetch from metalNetwork
				DestinationPrefixes: nw.DestinationPrefixes, // TODO would be better if we fetch from metalNetwork
				NetworkType:         networkType,
				NatType:             natType,
				Vrf:                 uint64(nw.Vrf),
				Asn:                 nw.ASN,
			})
		}

		allocationType, err := enum.GetEnum[apiv2.MachineAllocationType](strings.ToLower(string(alloc.Role)))
		if err != nil {
			return nil, err
		}

		if m.Allocation.Labels != nil {
			allocationLabels = &apiv2.Labels{
				Labels: m.Allocation.Labels,
			}
		}

		allocation = &apiv2.MachineAllocation{
			Uuid: alloc.UUID,
			Meta: &apiv2.Meta{
				CreatedAt: timestamppb.New(alloc.Created),
				Labels:    allocationLabels,
			},
			Name:             alloc.Name,
			Description:      alloc.Description,
			CreatedBy:        alloc.Creator,
			Project:          alloc.Project,
			Image:            apiv2Image,
			FilesystemLayout: filesystemLayout,
			Networks:         machineNetworks,
			Hostname:         alloc.Hostname,
			SshPublicKeys:    alloc.SSHPubKeys,
			Userdata:         alloc.UserData,
			AllocationType:   allocationType,
			FirewallRules:    firewallRules,
			DnsServer:        dnsServers,
			NtpServer:        ntpServers,
			Vpn:              vpn,
		}
	}

	stateString, err := enum.GetEnum[apiv2.MachineState](strings.ToLower(string(m.State.Value)))
	if err != nil {
		return nil, err
	}
	condition = &apiv2.MachineCondition{
		State:       apiv2.MachineState(stateString),
		Description: m.State.Description,
		Issuer:      m.State.Issuer,
	}

	event, err := r.s.ds.Event().Get(ctx, m.ID)
	if err != nil {
		return nil, err
	}

	liveliness, err := enum.GetEnum[apiv2.MachineLiveliness](strings.ToLower(string(event.Liveliness)))
	if err != nil {
		return nil, err
	}
	var (
		lastEventTime  *timestamppb.Timestamp
		lastErrorEvent *apiv2.MachineProvisioningEvent
		state          apiv2.MachineProvisioningEventState
		events         []*apiv2.MachineProvisioningEvent
	)
	if event.LastEventTime != nil {
		lastEventTime = timestamppb.New(*event.LastEventTime)
	}
	if event.LastErrorEvent != nil {
		eventType, err := enum.GetEnum[apiv2.MachineProvisioningEventType](event.LastErrorEvent.Message)
		if err != nil {
			return nil, err
		}
		lastErrorEvent = &apiv2.MachineProvisioningEvent{
			Time:  timestamppb.New(event.LastErrorEvent.Time),
			Event: eventType,
		}
		state, err = enum.GetEnum[apiv2.MachineProvisioningEventState](strings.ToLower(string(event.LastErrorEvent.Event)))
		if err != nil {
			return nil, err
		}
	}

	for _, e := range event.Events {
		eventType, err := enum.GetEnum[apiv2.MachineProvisioningEventType](string(e.Event))
		if err != nil {
			return nil, err
		}

		events = append(events, &apiv2.MachineProvisioningEvent{
			Time:    timestamppb.New(e.Time),
			Event:   eventType,
			Message: e.Message,
		})
	}

	recentEvents := &apiv2.MachineRecentProvisioningEvents{
		LastEventTime:  lastEventTime,
		LastErrorEvent: lastErrorEvent,
		Events:         events,
		State:          state,
	}

	status = &apiv2.MachineStatus{
		Condition:          condition,
		LedState:           &apiv2.MachineChassisIdentifyLEDState{},
		Liveliness:         liveliness,
		MetalHammerVersion: m.State.MetalHammerVersion,
	}

	result := &apiv2.Machine{
		Uuid: m.ID,
		Meta: &apiv2.Meta{
			CreatedAt: timestamppb.New(m.Created),
			UpdatedAt: timestamppb.New(m.Changed),
			Labels:    labels,
		},
		Partition:                apiv2Partition,
		Rack:                     m.RackID,
		Size:                     size,
		Hardware:                 hardware,
		Bios:                     bios,
		Allocation:               allocation,
		Status:                   status,
		RecentProvisioningEvents: recentEvents,
	}

	return result, nil
}

//---------------------------------------------------------------
// Write a function HandleXXXTask to handle the input task.
// Note that it satisfies the asynq.HandlerFunc interface.
//
// Handler doesn't need to be a function. You can define a type
// that satisfies asynq.Handler interface. See examples below.
//---------------------------------------------------------------

func (r *Store) MachineDeleteHandleFn(ctx context.Context, t *asynq.Task) error {
	// FIXME implement with machineDelete

	return nil
}

func (r *machineRepository) scopedMachineFilters(filter generic.EntityQuery) []generic.EntityQuery {
	var qs []generic.EntityQuery
	if r.scope != nil {
		qs = append(qs, queries.MachineProjectScoped(r.scope.projectID))
	}
	if filter != nil {
		qs = append(qs, filter)
	}
	return qs
}
