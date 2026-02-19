package repository

import (
	"context"
	"fmt"
	"sort"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/issues"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type partitionRepository struct {
	s *Store
}

func (p *partitionRepository) create(ctx context.Context, c *adminv2.PartitionServiceCreateRequest) (*metal.Partition, error) {
	partition, err := p.convertToInternal(ctx, c.Partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	resp, err := p.s.ds.Partition().Create(ctx, partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp, nil
}

func (p *partitionRepository) delete(ctx context.Context, e *metal.Partition) error {
	err := p.s.ds.Partition().Delete(ctx, e)
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}

func (p *partitionRepository) get(ctx context.Context, id string) (*metal.Partition, error) {
	partition, err := p.s.ds.Partition().Get(ctx, id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return partition, nil
}

func (p *partitionRepository) update(ctx context.Context, e *metal.Partition, req *adminv2.PartitionServiceUpdateRequest) (*metal.Partition, error) {
	if req.BootConfiguration != nil {
		e.BootConfiguration = metal.BootConfiguration{
			ImageURL:    req.BootConfiguration.ImageUrl,
			KernelURL:   req.BootConfiguration.KernelUrl,
			CommandLine: req.BootConfiguration.Commandline,
		}
	}

	if req.Description != nil {
		e.Description = *req.Description
	}
	if req.DnsServer != nil {
		servers := make(metal.DNSServers, 0, len(req.DnsServer))

		for _, s := range req.DnsServer {
			servers = append(servers, metal.DNSServer{
				IP: s.GetIp(),
			})
		}

		e.DNSServers = servers
	}
	if req.NtpServer != nil {
		servers := make(metal.NTPServers, 0, len(req.NtpServer))

		for _, s := range req.NtpServer {
			servers = append(servers, metal.NTPServer{
				Address: s.GetAddress(),
			})
		}

		e.NTPServers = servers
	}
	if len(req.MgmtServiceAddresses) == 1 {
		e.MgmtServiceAddress = req.MgmtServiceAddresses[0]
	}

	if req.Labels != nil {
		e.Labels = updateLabelsOnMap(req.Labels, e.Labels)
	}

	err := p.s.ds.Partition().Update(ctx, e)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return e, nil
}

func (p *partitionRepository) find(ctx context.Context, query *apiv2.PartitionQuery) (*metal.Partition, error) {
	partition, err := p.s.ds.Partition().Find(ctx, queries.PartitionFilter(query))
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return partition, nil
}

func (p *partitionRepository) list(ctx context.Context, query *apiv2.PartitionQuery) ([]*metal.Partition, error) {
	partitions, err := p.s.ds.Partition().List(ctx, queries.PartitionFilter(query))
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return partitions, nil
}

func (p *partitionRepository) matchScope(e *metal.Partition) bool {
	// Not Project Scoped
	return true
}

func (p *partitionRepository) convertToInternal(ctx context.Context, msg *apiv2.Partition) (*metal.Partition, error) {
	mgm := ""
	if len(msg.MgmtServiceAddresses) > 0 {
		// FIXME migrate metal model to slice as well
		mgm = msg.MgmtServiceAddresses[0]
	}
	var labels map[string]string
	if msg.Meta != nil && msg.Meta.Labels != nil {
		labels = msg.Meta.Labels.Labels
	}

	var (
		dnsServers metal.DNSServers
		ntpServers metal.NTPServers
	)
	for _, dnsServer := range msg.DnsServer {
		dnsServers = append(dnsServers, metal.DNSServer{
			IP: dnsServer.Ip,
		})
	}
	for _, ntpServer := range msg.NtpServer {
		ntpServers = append(ntpServers, metal.NTPServer{
			Address: ntpServer.Address,
		})
	}

	partition := &metal.Partition{
		Base: metal.Base{
			ID:          msg.Id,
			Name:        msg.Id,
			Description: msg.Description,
		},
		MgmtServiceAddress: mgm,
		Labels:             labels,
		DNSServers:         dnsServers,
		NTPServers:         ntpServers,
	}

	if msg.BootConfiguration != nil {
		partition.BootConfiguration = metal.BootConfiguration{
			ImageURL:    msg.BootConfiguration.ImageUrl,
			KernelURL:   msg.BootConfiguration.KernelUrl,
			CommandLine: msg.BootConfiguration.Commandline,
		}
	}

	if msg.Meta != nil {
		if msg.Meta.CreatedAt != nil {
			partition.Created = msg.Meta.CreatedAt.AsTime()
		}
		if msg.Meta.UpdatedAt != nil {
			partition.Changed = msg.Meta.UpdatedAt.AsTime()
		}
		partition.Generation = msg.Meta.Generation
	}

	return partition, nil
}

func (p *partitionRepository) convertToProto(ctx context.Context, e *metal.Partition) (*apiv2.Partition, error) {
	var (
		dnsServers []*apiv2.DNSServer
		ntpServers []*apiv2.NTPServer
	)
	for _, dnsServer := range e.DNSServers {
		dnsServers = append(dnsServers, &apiv2.DNSServer{
			Ip: dnsServer.IP,
		})
	}
	for _, ntpServer := range e.NTPServers {
		ntpServers = append(ntpServers, &apiv2.NTPServer{
			Address: ntpServer.Address,
		})
	}

	meta := &apiv2.Meta{
		CreatedAt:  timestamppb.New(e.Created),
		UpdatedAt:  timestamppb.New(e.Changed),
		Generation: e.Generation,
	}
	if e.Labels != nil {
		meta.Labels = &apiv2.Labels{Labels: e.Labels}
	}

	partition := &apiv2.Partition{
		Id:          e.ID,
		Description: e.Description,
		Meta:        meta,
		BootConfiguration: &apiv2.PartitionBootConfiguration{
			ImageUrl:    e.BootConfiguration.ImageURL,
			KernelUrl:   e.BootConfiguration.KernelURL,
			Commandline: e.BootConfiguration.CommandLine,
		},
		DnsServer: dnsServers,
		NtpServer: ntpServers,
	}
	return partition, nil
}

func (p *partitionRepository) Capacity(ctx context.Context, rq *adminv2.PartitionServiceCapacityRequest) (*adminv2.PartitionServiceCapacityResponse, error) {
	var (
		ms           []*metal.Machine
		ps           []*metal.Partition
		pcs          = make(map[string]*adminv2.PartitionCapacity)
		machineQuery = &apiv2.MachineQuery{}
	)

	if rq != nil && rq.Id != nil {
		p, err := p.s.ds.Partition().Get(ctx, *rq.Id)
		if err != nil {
			return nil, err
		}
		ps = append(ps, p)

		machineQuery.Partition = rq.Id
	} else {
		var err error
		ps, err = p.s.ds.Partition().List(ctx, queries.PartitionFilter(&apiv2.PartitionQuery{}))
		if err != nil {
			return nil, err
		}
	}

	// if filtered on partition get all without more filters for issues evaluation
	// to identify issues in requested machines, all machines are required.
	// One example is to find machines with duplicate bmc addresses.
	allMs, err := p.s.ds.Machine().List(ctx, queries.MachineFilter(machineQuery))
	if err != nil {
		return nil, err
	}

	if rq != nil && rq.Size != nil {
		// Ensure size exists
		resp, err := p.s.ds.Size().Get(ctx, *rq.Size)
		if err != nil {
			return nil, err
		}
		ms = lo.Filter(allMs, func(ms *metal.Machine, _ int) bool {
			return ms.SizeID == resp.ID
		})
	} else {
		ms = allMs
	}

	ecs, err := p.s.ds.Event().List(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch provisioning event containers: %w", err)
	}

	sizes, err := p.s.ds.Size().List(ctx, queries.SizeFilter(&apiv2.SizeQuery{}))
	if err != nil {
		return nil, fmt.Errorf("unable to list sizes: %w", err)
	}

	sizeReservations, err := p.s.ds.SizeReservation().List(ctx, queries.SizeReservationFilter(&apiv2.SizeReservationQuery{}))
	if err != nil {
		return nil, fmt.Errorf("unable to list size reservations: %w", err)
	}

	// Last event error is a minor issue that describes an unexpected transition in the provisioning cycle.
	// This happens quite often and you do not want to show these machines as unhealthy.
	// see https://metal-stack.io/docs/troubleshooting#last-event-error
	machinesWithIssues, err := issues.Find(&issues.Config{
		Machines:        allMs,
		EventContainers: ecs,
		Omit:            []issues.Type{issues.TypeLastEventError},
	})
	if err != nil {
		return nil, fmt.Errorf("unable to calculate machine issues: %w", err)
	}

	var (
		partitionsById         = metal.PartitionsByID(ps)
		ecsById                = make(map[string]*metal.ProvisioningEventContainer)
		sizesByID              = make(map[string]*metal.Size)
		sizeReservationsBySize = metal.SizeReservationsBySize(sizeReservations)
		machinesByProject      = make(map[string][]*metal.Machine)
	)

	// TODO future improvement implement metal.EventContainersById
	for _, ec := range ecs {
		ecsById[ec.ID] = ec
	}

	// TODO future improvement implement metal.SizesById
	for _, s := range sizes {
		sizesByID[s.ID] = s
	}

	for _, m := range ms {
		if m.Allocation == nil {
			continue
		}
		machinesByProject[m.Allocation.Project] = append(machinesByProject[m.Allocation.Project], m)
	}

	for _, m := range ms {

		ec, ok := ecsById[m.ID]
		if !ok {
			continue
		}

		part, ok := partitionsById[m.PartitionID]
		if !ok {
			continue
		}

		pc, ok := pcs[m.PartitionID]
		if !ok {
			pc = &adminv2.PartitionCapacity{
				Partition:             part.ID,
				MachineSizeCapacities: []*adminv2.MachineSizeCapacity{},
			}
		}
		pcs[m.PartitionID] = pc

		size, ok := sizesByID[m.SizeID]
		if !ok {
			size = metal.UnknownSize()
		}

		cap, ok := lo.Find(pc.MachineSizeCapacities, func(mc *adminv2.MachineSizeCapacity) bool {
			return mc.Size == size.ID
		})
		if !ok {
			cap = &adminv2.MachineSizeCapacity{
				Size: size.ID,
			}
			pc.MachineSizeCapacities = append(pc.MachineSizeCapacities, cap)
		}

		cap.Total++

		if _, ok := machinesWithIssues[m.ID]; ok {
			cap.Faulty++
			cap.FaultyMachines = append(cap.FaultyMachines, m.ID)
		}

		// allocation dependent counts
		switch {
		case m.Allocation != nil:
			cap.Allocated++
		case m.Waiting && !m.PreAllocated && m.State.Value == metal.AvailableState && ec.Liveliness == metal.MachineLivelinessAlive:
			// the free and allocatable machine counts consider the same aspects as the query for electing the machine candidate!
			cap.Allocatable++
			cap.Free++
		default:
			cap.Unavailable++
		}

		// provisioning state dependent counts
		switch pointer.FirstOrZero(ec.Events).Event {
		case metal.ProvisioningEventPhonedHome:
			cap.PhonedHome++
		case metal.ProvisioningEventWaiting:
			cap.Waiting++
		default:
			cap.Other++
			cap.OtherMachines = append(cap.OtherMachines, m.ID)
		}
	}

	var res []*adminv2.PartitionCapacity
	for _, pc := range pcs {
		for _, cap := range pc.MachineSizeCapacities {
			size, ok := sizesByID[cap.Size]
			if !ok {
				continue
			}

			rvs, ok := sizeReservationsBySize[size.ID]
			if !ok {
				continue
			}

			for _, reservation := range metal.SizeReservationsForPartition(rvs, pc.Partition) {
				machinesWithSizeAndPartition := lo.Filter(machinesByProject[reservation.ProjectID], func(m *metal.Machine, _ int) bool {
					return m.SizeID == size.ID && m.PartitionID == pc.Partition
				})
				usedReservations := min(len(machinesWithSizeAndPartition), reservation.Amount)

				cap.Reservations += int64(reservation.Amount)
				cap.UsedReservations += int64(usedReservations)

				if rq.Project != nil && *rq.Project == reservation.ProjectID {
					continue
				}

				cap.Free -= int64(reservation.Amount - usedReservations)
				cap.Free = max(cap.Free, 0)
			}
		}

		for _, cap := range pc.MachineSizeCapacities {
			cap.RemainingReservations = cap.Reservations - cap.UsedReservations
		}

		res = append(res, pc)
	}

	// Prevent flaky tests
	sort.SliceStable(res, func(i, j int) bool {
		return res[i].Partition < res[j].Partition
	})

	return &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: res}, nil
}
