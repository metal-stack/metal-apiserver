package repository

import (
	"context"
	"regexp"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	dnsName string = `^([a-zA-Z0-9_]{1}[a-zA-Z0-9_-]{0,62}){1}(\.[a-zA-Z0-9_]{1}[a-zA-Z0-9_-]{0,62})*[\._]?$`
)

var (
	regexDNSName = regexp.MustCompile(dnsName)
)

type partitionRepository struct {
	s *Store
}

// ValidateUpdate implements Partition.
func (p *partitionRepository) validateUpdate(ctx context.Context, req *adminv2.PartitionServiceUpdateRequest, _ *metal.Partition) error {
	partition := &apiv2.Partition{
		Id:                   req.Id,
		BootConfiguration:    req.BootConfiguration,
		DnsServer:            req.DnsServer,
		NtpServer:            req.NtpServer,
		MgmtServiceAddresses: req.MgmtServiceAddresses,
	}
	return validatePartition(ctx, partition)
}

// Create implements Partition.
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

// Delete implements Partition.
func (p *partitionRepository) delete(ctx context.Context, e *metal.Partition) error {
	err := p.s.ds.Partition().Delete(ctx, e)
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}

// Get implements Partition.
func (p *partitionRepository) get(ctx context.Context, id string) (*metal.Partition, error) {
	partition, err := p.s.ds.Partition().Get(ctx, id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return partition, nil
}

// Update implements Partition.
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

// Find implements Partition.
func (p *partitionRepository) find(ctx context.Context, query *apiv2.PartitionQuery) (*metal.Partition, error) {
	partition, err := p.s.ds.Partition().Find(ctx, queries.PartitionFilter(query))
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return partition, nil
}

// List implements Partition.
func (p *partitionRepository) list(ctx context.Context, query *apiv2.PartitionQuery) ([]*metal.Partition, error) {
	partitions, err := p.s.ds.Partition().List(ctx, queries.PartitionFilter(query))
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return partitions, nil
}

// MatchScope implements Partition.
func (p *partitionRepository) matchScope(e *metal.Partition) bool {
	// Not Project Scoped
	return true
}

// ConvertToInternal implements Partition.
func (p *partitionRepository) convertToInternal(ctx context.Context, msg *apiv2.Partition, opts ...Option) (*metal.Partition, error) {
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

// ConvertToProto implements Partition.
func (p *partitionRepository) convertToProto(ctx context.Context, e *metal.Partition, opts ...Option) (*apiv2.Partition, error) {
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
