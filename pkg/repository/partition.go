package repository

import (
	"context"
	"net"
	"net/netip"
	"regexp"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

const (
	dnsName string = `^([a-zA-Z0-9_]{1}[a-zA-Z0-9_-]{0,62}){1}(\.[a-zA-Z0-9_]{1}[a-zA-Z0-9_-]{0,62})*[\._]?$`
)

var (
	regexDNSName = regexp.MustCompile(dnsName)
)

type partitionRepository struct {
	r *Store
}

func validatePartition(ctx context.Context, partition *apiv2.Partition) error {
	if partition.Id == "" {
		return errorutil.InvalidArgument("partition id must not be empty")
	}
	if partition.BootConfiguration == nil {
		return errorutil.InvalidArgument("partition.bootconfiguration must not be nil")
	}
	if partition.BootConfiguration.ImageUrl == "" {
		return errorutil.InvalidArgument("partition.bootconfiguration.imageurl must not be empty")
	}
	if err := checkIfUrlExists(ctx, "partition imageurl of", partition.Id, partition.BootConfiguration.ImageUrl); err != nil {
		return errorutil.NewInvalidArgument(err)
	}
	if partition.BootConfiguration.KernelUrl == "" {
		return errorutil.InvalidArgument("partition.bootconfiguration.kernelurl must not be empty")
	}
	if err := checkIfUrlExists(ctx, "partition kernelurl of", partition.Id, partition.BootConfiguration.KernelUrl); err != nil {
		return errorutil.NewInvalidArgument(err)
	}

	if len(partition.DnsServer) > 3 {
		return errorutil.InvalidArgument("not more than 3 dnsservers must be specified")
	}
	for _, dns := range partition.DnsServer {
		_, err := netip.ParseAddr(dns.Ip)
		if err != nil {
			return errorutil.InvalidArgument("dnsserver ip is not valid:%w", err)
		}
	}

	if len(partition.NtpServer) > 5 {
		return errorutil.InvalidArgument("not more than 5 ntpservers must be specified")
	}
	for _, ntp := range partition.NtpServer {
		if net.ParseIP(ntp.Address) != nil {
			_, err := netip.ParseAddr(ntp.Address)
			if err != nil {
				return errorutil.InvalidArgument("ip: %s for ntp server not correct err: %w", ntp.Address, err)
			}
		} else {
			if !regexDNSName.MatchString(ntp.Address) {
				return errorutil.InvalidArgument("dns name: %s for ntp server not correct", ntp.Address)
			}
		}
	}

	return nil
}

// ValidateCreate implements Partition.
func (p *partitionRepository) ValidateCreate(ctx context.Context, req *adminv2.PartitionServiceCreateRequest) (*Validated[*adminv2.PartitionServiceCreateRequest], error) {
	partition := req.Partition
	err := validatePartition(ctx, partition)
	if err != nil {
		return nil, err
	}
	return &Validated[*adminv2.PartitionServiceCreateRequest]{
		message: req,
	}, nil
}

// ValidateDelete implements Partition.
func (p *partitionRepository) ValidateDelete(ctx context.Context, req *metal.Partition) (*Validated[*metal.Partition], error) {

	// FIXME all entities with partition relation must be deleted before

	return &Validated[*metal.Partition]{
		message: req,
	}, nil
}

// ValidateUpdate implements Partition.
func (p *partitionRepository) ValidateUpdate(ctx context.Context, req *adminv2.PartitionServiceUpdateRequest) (*Validated[*adminv2.PartitionServiceUpdateRequest], error) {
	partition := req.Partition
	err := validatePartition(ctx, partition)
	if err != nil {
		return nil, err
	}
	return &Validated[*adminv2.PartitionServiceUpdateRequest]{
		message: req,
	}, nil
}

// Create implements Partition.
func (p *partitionRepository) Create(ctx context.Context, c *Validated[*adminv2.PartitionServiceCreateRequest]) (*metal.Partition, error) {
	partition, err := p.ConvertToInternal(c.message.Partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	resp, err := p.r.ds.Partition().Create(ctx, partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp, nil
}

// Delete implements Partition.
func (p *partitionRepository) Delete(ctx context.Context, e *Validated[*metal.Partition]) (*metal.Partition, error) {
	partition, err := p.Get(ctx, e.message.ID)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	err = p.r.ds.Partition().Delete(ctx, partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return partition, nil
}

// Get implements Partition.
func (p *partitionRepository) Get(ctx context.Context, id string) (*metal.Partition, error) {
	partition, err := p.r.ds.Partition().Get(ctx, id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return partition, nil
}

// Update implements Partition.
func (p *partitionRepository) Update(ctx context.Context, req *Validated[*adminv2.PartitionServiceUpdateRequest]) (*metal.Partition, error) {
	partition := req.message.Partition

	old, err := p.Get(ctx, partition.Id)
	if err != nil {
		return nil, err
	}

	new, err := p.ConvertToInternal(partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	new.SetChanged(old.Changed)

	err = p.r.ds.Partition().Update(ctx, new)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return new, nil
}

// Find implements Partition.
func (p *partitionRepository) Find(ctx context.Context, query *apiv2.PartitionQuery) (*metal.Partition, error) {
	partition, err := p.r.ds.Partition().Find(ctx, queries.PartitionFilter(query))
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return partition, nil
}

// List implements Partition.
func (p *partitionRepository) List(ctx context.Context, query *apiv2.PartitionQuery) ([]*metal.Partition, error) {
	partitions, err := p.r.ds.Partition().List(ctx, queries.PartitionFilter(query))
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return partitions, nil
}

// MatchScope implements Partition.
func (p *partitionRepository) MatchScope(e *metal.Partition) error {
	// Not Project Scoped
	panic("unimplemented")
}

// ConvertToInternal implements Partition.
func (p *partitionRepository) ConvertToInternal(msg *apiv2.Partition) (*metal.Partition, error) {
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
		BootConfiguration: metal.BootConfiguration{
			ImageURL:    msg.BootConfiguration.ImageUrl,
			KernelURL:   msg.BootConfiguration.KernelUrl,
			CommandLine: msg.BootConfiguration.Commandline,
		},
		Labels:     labels,
		DNSServers: dnsServers,
		NTPServers: ntpServers,
	}

	return partition, nil
}

// ConvertToProto implements Partition.
func (p *partitionRepository) ConvertToProto(e *metal.Partition) (*apiv2.Partition, error) {
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

	meta := &apiv2.Meta{}
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
