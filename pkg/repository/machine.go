package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/avast/retry-go/v4"
	"github.com/hibiken/asynq"
	"github.com/metal-stack/api/go/enum"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/fsm"
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

func (r *machineRepository) Dhcp(ctx context.Context, req *infrav2.BootServiceDhcpRequest) (*infrav2.BootServiceDhcpResponse, error) {
	err := r.SendEvent(ctx, r.s.log, req.Uuid, &infrav2.MachineProvisioningEvent{
		Time:    timestamppb.Now(),
		Event:   infrav2.ProvisioningEventType_PROVISIONING_EVENT_TYPE_PXE_BOOTING,
		Message: "machine sent extended dhcp request",
	})
	if err != nil {
		return nil, err
	}
	machine, err := r.get(ctx, req.Uuid)
	if err != nil {
		return nil, err
	}

	if machine.PartitionID == "" {
		machine.PartitionID = req.Partition
		err := r.s.ds.Machine().Update(ctx, machine)
		if err != nil {
			return nil, err
		}
	}
	return &infrav2.BootServiceDhcpResponse{}, nil
}

func (r *machineRepository) SetMachineConnectedToVPN(ctx context.Context, id string, connected bool, ips []string) (*apiv2.Machine, error) {
	m, err := r.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.Allocation == nil {
		return nil, errorutil.InvalidArgument("machine is not allocated")
	}
	if m.Allocation.VPN == nil {
		return nil, errorutil.InvalidArgument("machine is not configured for VPN")
	}
	m.Allocation.VPN.Connected = connected
	m.Allocation.VPN.IPs = ips

	err = r.s.ds.Machine().Update(ctx, m)
	if err != nil {
		return nil, err
	}
	return r.convertToProto(ctx, m)
}

func (r *machineRepository) SendEvent(ctx context.Context, log *slog.Logger, machineID string, event *infrav2.MachineProvisioningEvent) error {
	if event == nil {
		return errorutil.InvalidArgument("event for machine %s is nil", machineID)
	}

	_, err := r.find(ctx, &apiv2.MachineQuery{Uuid: &machineID})
	if err != nil && !errorutil.IsNotFound(err) {
		return err
	}

	// an event can actually create an empty machine. This enables us to also catch the very first PXE Booting event
	// in a machine lifecycle
	if errorutil.IsNotFound(err) {
		_, err := r.s.ds.Machine().Create(ctx, &metal.Machine{Base: metal.Base{ID: machineID}})
		if err != nil {
			return err
		}
	}

	eventType, err := metal.ToProvisioningEventType(event.Event)
	if err != nil {
		return err
	}

	ev := &metal.ProvisioningEvent{
		Time:    time.Now(),
		Event:   eventType,
		Message: event.Message,
	}

	ec, err := r.s.ds.Event().Find(ctx, queries.EventFilter(machineID))
	if err != nil && !errorutil.IsNotFound(err) {
		return err
	}

	if ec == nil {
		ec = &metal.ProvisioningEventContainer{
			Base: metal.Base{
				ID: machineID,
			},
			Liveliness: metal.MachineLivelinessAlive,
		}
	}

	newEC, err := fsm.HandleProvisioningEvent(ctx, log, ec, ev)
	if err != nil {
		return err
	}

	if err = newEC.Validate(); err != nil {
		return err
	}
	newEC.TrimEvents(100)

	return r.s.ds.Event().Upsert(ctx, newEC)
}

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

	allocationProject := pointer.SafeDeref(pointer.SafeDeref(machine).Allocation).Project
	return r.scope.projectID == allocationProject
}

func (r *machineRepository) create(ctx context.Context, req *apiv2.MachineServiceCreateRequest) (*metal.Machine, error) {
	// TODO if allocation was created, create a new queue entry for the Wait endpoint like so:
	// err := r.s.queue.PushMachineAllocation(ctx, m, task.MachineAllocationPayload{UUID: m})
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

	if len(req.SshPublicKeys) > 0 {
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
	info, err := r.s.task.NewMachineDeleteTask(uuid, allocationUUID)
	if err != nil {
		return err
	}

	r.s.log.Info("machine delete queued", "info", info)

	return nil
}

func (r *machineRepository) find(ctx context.Context, rq *apiv2.MachineQuery) (*metal.Machine, error) {
	ms, err := r.s.ds.Machine().Find(ctx, r.scopedMachineFilters(queries.MachineFilter(rq))...)
	if err != nil {
		return nil, err
	}

	return ms, nil
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

func (r *machineRepository) convertToInternal(ctx context.Context, machine *apiv2.Machine) (*metal.Machine, error) {
	panic("unimplemented")
}

func (r *machineRepository) convertToProto(ctx context.Context, m *metal.Machine) (*apiv2.Machine, error) {
	var (
		labels           *apiv2.Labels
		allocationLabels *apiv2.Labels
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

	// For machines which are initially created there is no known partition yet
	partition, err := r.s.Partition().Get(ctx, m.PartitionID)
	if err != nil && !errorutil.IsNotFound(err) {
		return nil, err
	}
	// For machines which are initially created there is no known size yet
	size, err = r.s.Size().Get(ctx, m.SizeID)
	if err != nil {
		if !errorutil.IsNotFound(err) {
			return nil, err
		}
		size = r.s.Size().AdditionalMethods().unknownSize()
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
				Hostname:   neigh.Hostname,
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
		})
	}

	hardware := &apiv2.MachineHardware{
		Memory: m.Hardware.Memory,
		Disks:  disks,
		Cpus:   cpus,
		Gpus:   gpus,
		Nics:   nics,
	}

	if m.Allocation != nil {
		alloc := m.Allocation

		image, err := r.s.Image().Get(ctx, alloc.ImageID)
		if err != nil {
			return nil, err
		}

		if alloc.FilesystemLayout != nil {
			filesystemLayout, err = r.s.FilesystemLayout().Get(ctx, alloc.FilesystemLayout.ID)
			if err != nil {
				return nil, err
			}
		}

		if alloc.VPN != nil {
			vpn = &apiv2.MachineVPN{
				ControlPlaneAddress: alloc.VPN.ControlPlaneAddress,
				AuthKey:             alloc.VPN.AuthKey,
				Connected:           alloc.VPN.Connected,
				Ips:                 alloc.VPN.IPs,
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
			metalNetwork, err := r.s.ds.Network().Get(ctx, nw.NetworkID)
			if err != nil {
				return nil, err
			}
			if metalNetwork.NetworkType == nil {
				return nil, errorutil.Internal("network type of network:%s is nil", nw.NetworkID)
			}
			networkType, err := metal.FromNetworkType(*metalNetwork.NetworkType)
			if err != nil {
				return nil, err
			}
			if metalNetwork.NATType == nil {
				return nil, errorutil.Internal("nat type of network:%s is nil", nw.NetworkID)
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
			Image:            image,
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
		eventType, err := enum.GetEnum[apiv2.MachineProvisioningEventType](event.LastErrorEvent.Event.String())
		if err != nil {
			return nil, err
		}
		lastErrorEvent = &apiv2.MachineProvisioningEvent{
			Time:  timestamppb.New(event.LastErrorEvent.Time),
			Event: eventType,
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
			CreatedAt:  timestamppb.New(m.Created),
			UpdatedAt:  timestamppb.New(m.Changed),
			Labels:     labels,
			Generation: m.Generation,
		},
		Partition:                partition,
		Rack:                     m.RackID,
		Size:                     size,
		Hardware:                 hardware,
		Allocation:               allocation,
		Status:                   status,
		RecentProvisioningEvents: recentEvents,
	}

	return result, nil
}

func (r *machineRepository) Register(ctx context.Context, req *infrav2.BootServiceRegisterRequest) (*metal.Machine, error) {
	if req.Uuid == "" {
		return nil, errorutil.InvalidArgument("uuid is empty")
	}
	if req.Hardware == nil {
		return nil, errorutil.InvalidArgument("hardware is nil")
	}
	if req.Bios == nil {
		return nil, errorutil.InvalidArgument("bios is nil")
	}

	m, err := r.s.ds.Machine().Get(ctx, req.Uuid)
	if err != nil && !errorutil.IsNotFound(err) {
		return nil, err
	}
	// TODO changed behavior compared to metal-api, create machine here if not already done during dhcp request
	if m == nil {
		m, err = r.s.ds.Machine().Create(ctx, &metal.Machine{Base: metal.Base{ID: req.Uuid}})
		if err != nil {
			return nil, err
		}
	}

	disks := []metal.BlockDevice{}
	for i := range req.Hardware.Disks {
		d := req.Hardware.Disks[i]
		disks = append(disks, metal.BlockDevice{
			Name: d.Name,
			Size: d.Size,
		})
	}

	nics := metal.Nics{}
	for _, nic := range req.Hardware.Nics {
		neighs := metal.Nics{}
		for _, neigh := range nic.Neighbors {
			neighs = append(neighs, metal.Nic{
				Name:       neigh.Name,
				MacAddress: neigh.Mac,
				Hostname:   neigh.Hostname,
				Identifier: neigh.Identifier,
			})
		}
		nics = append(nics, metal.Nic{
			Name:       nic.Name,
			MacAddress: nic.Mac,
			Identifier: nic.Identifier,
			Neighbors:  neighs,
		})
	}

	cpus := []metal.MetalCPU{}
	for _, cpu := range req.Hardware.Cpus {
		cpus = append(cpus, metal.MetalCPU{
			Vendor:  cpu.Vendor,
			Model:   cpu.Model,
			Cores:   cpu.Cores,
			Threads: cpu.Threads,
		})
	}

	gpus := []metal.MetalGPU{}
	for _, gpu := range req.Hardware.Gpus {
		gpus = append(gpus, metal.MetalGPU{
			Vendor: gpu.Vendor,
			Model:  gpu.Model,
		})
	}

	machineHardware := metal.MachineHardware{
		Memory:    req.Hardware.Memory,
		Disks:     disks,
		Nics:      nics,
		MetalCPUs: cpus,
		MetalGPUs: gpus,
	}

	size, err := r.s.Size().AdditionalMethods().FromHardware(ctx, machineHardware)
	if err != nil {
		size = metal.UnknownSize()
		r.s.log.Error("no size found for hardware, defaulting to unknown size", "hardware", machineHardware, "error", err)
	}

	var ipmi metal.IPMI
	if req.Bmc != nil {
		bmc := req.Bmc

		ipmi = metal.IPMI{
			Address:     bmc.Address,
			MacAddress:  bmc.Mac,
			User:        bmc.User,
			Password:    bmc.Password,
			Interface:   bmc.Interface,
			BMCVersion:  bmc.Version,
			PowerState:  bmc.PowerState,
			LastUpdated: time.Now(),
		}
	}

	if req.Fru != nil {
		ipmi.Fru = metal.Fru{
			ChassisPartNumber:   pointer.SafeDeref(req.Fru.ChassisPartNumber),
			ChassisPartSerial:   pointer.SafeDeref(req.Fru.ChassisPartSerial),
			BoardMfg:            pointer.SafeDeref(req.Fru.BoardMfg),
			BoardMfgSerial:      pointer.SafeDeref(req.Fru.BoardMfgSerial),
			BoardPartNumber:     pointer.SafeDeref(req.Fru.BoardPartNumber),
			ProductManufacturer: pointer.SafeDeref(req.Fru.ProductManufacturer),
			ProductPartNumber:   pointer.SafeDeref(req.Fru.ProductPartNumber),
			ProductSerial:       pointer.SafeDeref(req.Fru.ProductSerial),
		}
	}

	if m == nil {
		// machine is not in the database, create it
		m = &metal.Machine{
			Base: metal.Base{
				ID: req.Uuid,
			},
			Allocation: nil,
			SizeID:     size.ID,
			Hardware:   machineHardware,
			BIOS: metal.BIOS{
				Version: req.Bios.Version,
				Vendor:  req.Bios.Vendor,
				Date:    req.Bios.Date,
			},
			State: metal.MachineState{
				Value:              metal.AvailableState,
				MetalHammerVersion: req.MetalHammerVersion,
			},
			LEDState: metal.ChassisIdentifyLEDState{
				Value:       metal.LEDStateOff,
				Description: "Machine registered",
			},
			Tags:        req.Tags,
			IPMI:        ipmi,
			PartitionID: req.Partition,
		}

		_, err := r.s.ds.Machine().Create(ctx, m)
		if err != nil {
			return nil, err
		}
	} else {
		// machine has already registered, update it
		m.SizeID = size.ID
		m.Hardware = machineHardware
		m.BIOS.Version = req.Bios.Version
		m.BIOS.Vendor = req.Bios.Vendor
		m.BIOS.Date = req.Bios.Date
		m.IPMI = ipmi
		m.State.MetalHammerVersion = req.MetalHammerVersion
		m.PartitionID = req.Partition

		err = r.s.ds.Machine().Update(ctx, m)
		if err != nil {
			return nil, err
		}
	}

	ec, err := r.s.ds.Event().Find(ctx, queries.EventFilter(m.ID))
	if err != nil && !errorutil.IsNotFound(err) {
		return nil, err
	}
	if ec == nil {
		_, err = r.s.ds.Event().Create(ctx, &metal.ProvisioningEventContainer{
			Base: metal.Base{
				ID: m.ID,
			},
			Events: metal.ProvisioningEvents{
				{
					Event:   metal.ProvisioningEventAlive,
					Time:    time.Now(),
					Message: "machine registered",
				},
			},
		},
		)
		if err != nil {
			return nil, err
		}
	}

	// TODO migrate to task
	err = retry.Do(
		func() error {
			machine, err := r.convertToProto(ctx, m)
			if err != nil {
				return err
			}
			err = r.s.Switch().AdditionalMethods().ConnectMachineWithSwitches(ctx, machine)
			if err != nil {
				return err
			}
			m.RackID = machine.Rack
			return r.s.ds.Machine().Update(ctx, m)
		},
		retry.Attempts(10),
		retry.RetryIf(func(err error) bool {
			return errorutil.IsConflict(err)
		}),
		retry.DelayType(retry.CombineDelay(retry.BackOffDelay, retry.RandomDelay)),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (r *machineRepository) InstallationSucceeded(ctx context.Context, req *infrav2.BootServiceInstallationSucceededRequest) (*metal.Machine, error) {
	m, err := r.s.ds.Machine().Get(ctx, req.Uuid)
	if err != nil {
		return nil, err
	}
	if m.Allocation == nil {
		return nil, fmt.Errorf("the machine %q is not allocated", req.Uuid)
	}

	m.Allocation.ConsolePassword = req.ConsolePassword

	err = r.s.ds.Machine().Update(ctx, m)
	if err != nil {
		return nil, err
	}

	vrf := ""
	switch role := m.Allocation.Role; role {
	case metal.RoleFirewall:
		// firewalls are not connected into tenant vrfs
		vrf = "default"
	case metal.RoleMachine:
		// TODO machineNetworks still are old-fashioned
		for _, mn := range m.Allocation.MachineNetworks {
			if mn.Private {
				vrf = fmt.Sprintf("vrf%d", mn.Vrf)
				break
			}
		}
	default:
		return nil, fmt.Errorf("unknown allocation role:%q found", role)
	}
	if vrf == "" {
		return nil, fmt.Errorf("the machine %q could not be put into the vrf because no vrf was found, error: %w", req.Uuid, err)
	}

	// TODO convert to tasks
	err = retry.Do(
		func() error {
			_, err := r.s.Switch().AdditionalMethods().SetVrfAtSwitches(ctx, m, vrf)
			return err
		},
		retry.Attempts(10),
		retry.RetryIf(func(err error) bool {
			return errorutil.IsConflict(err)
		}),
		retry.DelayType(retry.CombineDelay(retry.BackOffDelay, retry.RandomDelay)),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		return nil, fmt.Errorf("the machine %q could not be enslaved into the vrf %s, error: %w", req.Uuid, vrf, err)
	}

	err = r.MachineBMCCommand(ctx, m.ID, m.PartitionID, apiv2.MachineBMCCommand_MACHINE_BMC_COMMAND_MACHINE_CREATED)
	if err != nil {
		return nil, fmt.Errorf("unable to send machinecommand to trigger boot to disk %w", err)
	}

	return m, nil
}

func (r *machineRepository) UpdateBMCInfo(ctx context.Context, req *infrav2.UpdateBMCInfoRequest) (*infrav2.UpdateBMCInfoResponse, error) {
	if req.Partition == "" {
		return nil, errorutil.InvalidArgument("partition id must not be empty")
	}

	partition, err := r.s.ds.Partition().Get(ctx, req.Partition)
	if err != nil {
		return nil, err
	}

	ms, err := r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{Partition: &partition.ID}))
	if err != nil {
		return nil, err
	}

	known := make(map[string]string)
	for _, m := range ms {
		uuid := m.ID
		if uuid == "" {
			continue
		}
		known[uuid] = m.IPMI.Address
	}
	resp := &infrav2.UpdateBMCInfoResponse{
		UpdatedMachines: []string{},
		CreatedMachines: []string{},
	}
	// create empty machines for uuids that are not yet known to the metal-api
	for uuid, report := range req.BmcReports {
		if uuid == "" {
			continue
		}
		if _, ok := known[uuid]; ok {
			continue
		}
		m := &metal.Machine{
			Base: metal.Base{
				ID: uuid,
			},
			PartitionID: partition.ID,
			IPMI: metal.IPMI{
				Address:     report.Bmc.Address,
				LastUpdated: time.Now(),
				MacAddress:  report.Bmc.Mac,
			},
		}
		ledstate, err := metal.LEDStateFrom(report.LedState.Value)
		if err == nil {
			m.LEDState = metal.ChassisIdentifyLEDState{
				Value: ledstate,
			}
		} else {
			r.s.log.Error("unable to decode ledstate", "id", uuid, "ledstate", report.LedState.Value, "error", err)
		}
		_, err = r.s.ds.Machine().Create(ctx, m)
		if err != nil {
			r.s.log.Error("could not create machine", "id", uuid, "ipmi-ip", report.Bmc.Address, "m", m, "err", err)
			continue
		}
		resp.CreatedMachines = append(resp.CreatedMachines, uuid)
	}
	// update machine bmc data if bmc ip changed
	for _, machine := range ms {
		uuid := machine.ID
		if uuid == "" {
			continue
		}
		// if oldmachine.uuid is not part of this update cycle skip it
		report, ok := req.BmcReports[uuid]
		if !ok {
			continue
		}

		// machine was created by a PXE boot event and has no partition set.
		if machine.PartitionID == "" {
			machine.PartitionID = partition.ID
		}

		if machine.PartitionID != partition.ID {
			r.s.log.Error("could not update machine because overlapping id found", "id", uuid, "machine", machine, "partition", req.Partition)
			continue
		}

		// FRU
		if report.Fru != nil {
			fru := report.Fru
			machine.IPMI.Fru.ChassisPartSerial = pointer.SafeDerefOrDefault(fru.ChassisPartSerial, machine.IPMI.Fru.ChassisPartSerial)
			machine.IPMI.Fru.ChassisPartNumber = pointer.SafeDerefOrDefault(fru.ChassisPartNumber, machine.IPMI.Fru.ChassisPartNumber)
			machine.IPMI.Fru.BoardMfg = pointer.SafeDerefOrDefault(fru.BoardMfg, machine.IPMI.Fru.BoardMfg)
			machine.IPMI.Fru.BoardMfgSerial = pointer.SafeDerefOrDefault(fru.BoardMfgSerial, machine.IPMI.Fru.BoardMfgSerial)
			machine.IPMI.Fru.BoardPartNumber = pointer.SafeDerefOrDefault(fru.BoardPartNumber, machine.IPMI.Fru.BoardPartNumber)
			machine.IPMI.Fru.ProductManufacturer = pointer.SafeDerefOrDefault(fru.ProductManufacturer, machine.IPMI.Fru.ProductManufacturer)
			machine.IPMI.Fru.ProductSerial = pointer.SafeDerefOrDefault(fru.ProductSerial, machine.IPMI.Fru.ProductSerial)
			machine.IPMI.Fru.ProductPartNumber = pointer.SafeDerefOrDefault(fru.ProductPartNumber, machine.IPMI.Fru.ProductPartNumber)
		}

		if report.Bios != nil {
			if report.Bios.Version != "" {
				machine.BIOS.Version = report.Bios.Version
			}
			if report.Bios.Vendor != "" {
				machine.BIOS.Vendor = report.Bios.Vendor
			}
			if report.Bios.Date != "" {
				machine.BIOS.Date = report.Bios.Date
			}
		}

		if report.Bmc != nil {
			if report.Bmc.Version != "" {
				machine.IPMI.BMCVersion = report.Bmc.Version
			}
			if report.Bmc.Address != "" {
				machine.IPMI.Address = report.Bmc.Address
			}
			if report.Bmc.Interface != "" {
				machine.IPMI.Interface = report.Bmc.Interface
			}
			if report.Bmc.Mac != "" {
				machine.IPMI.MacAddress = report.Bmc.Mac
			}
			if report.Bmc.User != "" {
				machine.IPMI.User = report.Bmc.User
			}
			if report.Bmc.Password != "" {
				machine.IPMI.Password = report.Bmc.Password
			}
			if report.Bmc.PowerState != "" {
				machine.IPMI.PowerState = report.Bmc.PowerState
			}
		}

		if report.PowerMetric != nil {
			machine.IPMI.PowerMetric = &metal.PowerMetric{
				AverageConsumedWatts: report.PowerMetric.AverageConsumedWatts,
				IntervalInMin:        report.PowerMetric.IntervalInMin,
				MaxConsumedWatts:     report.PowerMetric.MaxConsumedWatts,
				MinConsumedWatts:     report.PowerMetric.MinConsumedWatts,
			}
		}
		var powerSupplies metal.PowerSupplies
		for _, ps := range report.PowerSupplies {
			powerSupplies = append(powerSupplies, metal.PowerSupply{
				Status: metal.PowerSupplyStatus{
					Health: ps.Health,
					State:  ps.State,
				},
			})
		}
		machine.IPMI.PowerSupplies = powerSupplies

		if report.LedState != nil {
			ledstate, err := metal.LEDStateFrom(report.LedState.Value)
			if err == nil {
				machine.LEDState = metal.ChassisIdentifyLEDState{
					Value:       ledstate,
					Description: machine.LEDState.Description,
				}
			} else {
				r.s.log.Error("unable to decode ledstate", "id", uuid, "ledstate", report.LedState.Value, "error", err)
			}
		}
		machine.IPMI.LastUpdated = time.Now()

		err = r.s.ds.Machine().Update(ctx, machine)
		if err != nil {
			r.s.log.Error("could not update machine", "id", uuid, "ip", report.Bmc.Address, "machine", machine, "err", err)
			continue
		}
		resp.UpdatedMachines = append(resp.UpdatedMachines, uuid)
	}
	return resp, nil
}

func (r *machineRepository) GetBMC(ctx context.Context, req *adminv2.MachineServiceGetBMCRequest) (*adminv2.MachineServiceGetBMCResponse, error) {
	machine, err := r.s.ds.Machine().Get(ctx, req.Uuid)
	if err != nil {
		return nil, err
	}

	bmcReport := r.convertToBMCReport(machine)

	return &adminv2.MachineServiceGetBMCResponse{
		Uuid: req.Uuid,
		Bmc:  bmcReport,
	}, nil
}

func (r *machineRepository) ListBMC(ctx context.Context, req *adminv2.MachineServiceListBMCRequest) (*adminv2.MachineServiceListBMCResponse, error) {
	machines, err := r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{Bmc: req.Query}))
	if err != nil {
		return nil, err
	}

	bmcReports := make(map[string]*apiv2.MachineBMCReport)
	for _, machine := range machines {
		bmcReports[machine.ID] = r.convertToBMCReport(machine)
	}

	return &adminv2.MachineServiceListBMCResponse{
		BmcReports: bmcReports,
	}, nil
}

func (r *machineRepository) convertToBMCReport(machine *metal.Machine) *apiv2.MachineBMCReport {
	var (
		bmc           *apiv2.MachineBMC
		bios          *apiv2.MachineBios
		fru           *apiv2.MachineFRU
		powerMetric   *apiv2.MachinePowerMetric
		powerSupplies = []*apiv2.MachinePowerSupply{}
	)

	for _, ps := range machine.IPMI.PowerSupplies {
		powerSupplies = append(powerSupplies, &apiv2.MachinePowerSupply{
			Health: ps.Status.Health,
			State:  ps.Status.State,
		})
	}

	if machine.IPMI.PowerMetric != nil {
		powerMetric = &apiv2.MachinePowerMetric{
			AverageConsumedWatts: machine.IPMI.PowerMetric.AverageConsumedWatts,
			MaxConsumedWatts:     machine.IPMI.PowerMetric.MaxConsumedWatts,
			MinConsumedWatts:     machine.IPMI.PowerMetric.MinConsumedWatts,
			IntervalInMin:        machine.IPMI.PowerMetric.IntervalInMin,
		}
	}

	fru = &apiv2.MachineFRU{
		ChassisPartNumber:   &machine.IPMI.Fru.ChassisPartNumber,
		ChassisPartSerial:   &machine.IPMI.Fru.ChassisPartSerial,
		BoardMfg:            &machine.IPMI.Fru.BoardMfg,
		BoardMfgSerial:      &machine.IPMI.Fru.BoardMfgSerial,
		BoardPartNumber:     &machine.IPMI.Fru.BoardPartNumber,
		ProductManufacturer: &machine.IPMI.Fru.ProductManufacturer,
		ProductPartNumber:   &machine.IPMI.Fru.ProductPartNumber,
		ProductSerial:       &machine.IPMI.Fru.ProductSerial,
	}

	bmc = &apiv2.MachineBMC{
		Address:    machine.IPMI.Address,
		Mac:        machine.IPMI.MacAddress,
		User:       machine.IPMI.User,
		Password:   machine.IPMI.Password,
		Interface:  machine.IPMI.Interface,
		Version:    machine.IPMI.BMCVersion,
		PowerState: machine.IPMI.PowerState,
	}

	bios = &apiv2.MachineBios{
		Version: machine.BIOS.Version,
		Vendor:  machine.BIOS.Vendor,
		Date:    machine.BIOS.Date,
	}

	bmcReport := &apiv2.MachineBMCReport{
		Bmc:           bmc,
		Bios:          bios,
		Fru:           fru,
		PowerMetric:   powerMetric,
		PowerSupplies: powerSupplies,
		LedState: &apiv2.MachineChassisIdentifyLEDState{
			Value:       string(machine.LEDState.Value),
			Description: machine.LEDState.Description,
		},
		UpdatedAt: timestamppb.New(machine.IPMI.LastUpdated),
	}

	return bmcReport
}

func (r *machineRepository) MachineBMCCommand(ctx context.Context, machineUUID, partition string, command apiv2.MachineBMCCommand) error {
	cmdString, err := enum.GetStringValue(command)
	if err != nil {
		return err
	}

	info, err := r.s.task.NewMachineBMCCommandTask(machineUUID, partition, *cmdString)
	if err != nil {
		return err
	}
	r.s.log.Debug("machine bmc command scheduled", "task", info)
	return nil
}

func (r *machineRepository) Wait(ctx context.Context, req *infrav2.BootServiceWaitRequest, srv *connect.ServerStream[infrav2.BootServiceWaitResponse]) error {
	// FIXME completely untested
	machineID := req.Uuid
	r.s.log.Info("wait for allocation called by", "machineID", machineID)

	m, err := r.s.ds.Machine().Get(ctx, machineID)
	if err != nil {
		return err
	}

	if m.Allocation != nil {
		return nil
	}

	// machine is not yet allocated, so we set the waiting flag
	m.Waiting = true
	err = r.s.ds.Machine().Update(ctx, m)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			return
		}
		// TODO This is prone to fail with optlock, either retry or async task
		m.Waiting = false
		err = r.s.ds.Machine().Update(ctx, m)
		if err != nil {
			r.s.log.Error("unable to remove waiting flag from machine", "machineID", machineID, "error", err)
		}
	}()

	allocationChan := r.s.queue.WaitMachineAllocation(ctx, machineID)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case msg, ok := <-allocationChan:
		if !ok {
			return errorutil.Internal("machine allocation channel is empty")
		}
		if msg.UUID != machineID {
			return errorutil.Internal("machine %s does not match channel response %s", machineID, msg)
		}
		machine, err := r.s.UnscopedMachine().Get(ctx, machineID)
		if err != nil {
			return err
		}
		if m.Allocation == nil {
			return errorutil.Internal("machine %s is not allocated", machineID)
		}
		err = srv.Send(&infrav2.BootServiceWaitResponse{
			Allocation: machine.Allocation,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *machineRepository) WaitForBMCCommand(ctx context.Context, req *infrav2.WaitForBMCCommandRequest, stream *connect.ServerStream[infrav2.WaitForBMCCommandResponse]) error {

	// Stream messages to client
	cmdChan := r.s.queue.WaitMachineCommand(ctx, req.Partition)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-cmdChan:
			if !ok {
				return nil
			}

			r.s.log.Debug("waitformachineevent received", "message", msg)

			m, err := r.s.ds.Machine().Get(ctx, msg.UUID)
			if err != nil {
				return errorutil.Internal("unable to get machine for machinecommand %w", err)
			}
			cmd, err := enum.GetEnum[apiv2.MachineBMCCommand](msg.Command)
			if err != nil {
				return errorutil.Internal("unable to decode machinecommand %w", err)
			}
			ipmi := m.IPMI
			resp := &infrav2.WaitForBMCCommandResponse{
				Uuid:       m.ID,
				BmcCommand: cmd,
				CommandId:  msg.CommandID,
				MachineBmc: &apiv2.MachineBMC{
					Address:  ipmi.Address,
					User:     ipmi.User,
					Password: ipmi.Password,
				},
			}

			err = stream.Send(resp)
			if err != nil {
				return errorutil.Internal("unable to send response into stream %w", err)
			}

			r.s.log.Debug("waitformachineevent sent to stream", "response", resp)
		}
	}
}

func (r *machineRepository) BMCCommandDone(ctx context.Context, req *infrav2.BMCCommandDoneRequest) (*infrav2.BMCCommandDoneResponse, error) {
	err := r.s.queue.PushMachineCommandDone(ctx, req.CommandId, task.BMCCommandDonePayload{Error: req.Error})
	if err != nil {
		return nil, err
	}
	return &infrav2.BMCCommandDoneResponse{}, nil
}

func (r *machineRepository) GetConsolePassword(ctx context.Context, machineUUID string) (string, error) {
	m, err := r.s.ds.Machine().Get(ctx, machineUUID)
	if err != nil {
		return "", err
	}
	if m.Allocation == nil {
		return "", errorutil.FailedPrecondition("machine %s is not allocated", machineUUID)
	}

	return m.Allocation.ConsolePassword, nil
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

func (r *Store) MachineBMCCommandHandleFn(ctx context.Context, t *asynq.Task) error {
	var payload task.MachineBMCCommandPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %w %w", err, asynq.SkipRetry)
	}
	r.log.Info("machine bmc command handler", "machine", payload.UUID, "command", payload.Command)

	if err := r.queue.PushMachineCommand(ctx, payload.Partition, payload); err != nil {
		return err
	}

	select {
	case result := <-r.queue.WaitMachineCommandDone(ctx, payload.CommandID):
		r.log.Debug("machine bmc command handler done received", "result", result)
		if result.Error != nil {
			if _, writeErr := t.ResultWriter().Write([]byte(*result.Error)); writeErr != nil {
				r.log.Warn("machine bmc command handler could not command execution error to task result", "error", writeErr)
			}
			return errors.New(*result.Error)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
