package repository

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"regexp"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
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
		return fmt.Errorf("partition id must not be empty")
	}
	if partition.BootConfiguration == nil {
		return fmt.Errorf("partition.bootconfiguration must not be nil")
	}
	if partition.BootConfiguration.ImageUrl == "" {
		return fmt.Errorf("partition.bootconfiguration.imageurl must not be empty")
	}
	if err := checkIfUrlExists(ctx, "partition imageurl of", partition.Id, partition.BootConfiguration.ImageUrl); err != nil {
		return err
	}
	if partition.BootConfiguration.KernelUrl == "" {
		return fmt.Errorf("partition.bootconfiguration.kernelurl must not be empty")
	}
	if err := checkIfUrlExists(ctx, "partition kernelurl of", partition.Id, partition.BootConfiguration.KernelUrl); err != nil {
		return err
	}

	if len(partition.DnsServer) > 3 {
		return fmt.Errorf("not more than 3 dnsservers must be specified")
	}
	for _, dns := range partition.DnsServer {
		_, err := netip.ParseAddr(dns.Ip)
		if err != nil {
			return fmt.Errorf("dnsserver ip is not valid:%w", err)
		}
	}

	if len(partition.NtpServer) > 5 {
		return fmt.Errorf("not more than 5 ntpservers must be specified")
	}
	for _, ntp := range partition.NtpServer {
		if net.ParseIP(ntp.Address) != nil {
			_, err := netip.ParseAddr(ntp.Address)
			if err != nil {
				return fmt.Errorf("ip: %s for ntp server not correct err: %w", ntp.Address, err)
			}
		} else {
			if !regexDNSName.MatchString(ntp.Address) {
				return fmt.Errorf("dns name: %s for ntp server not correct", ntp.Address)
			}
		}
	}

	return nil
}

// ValidateCreate implements Partition.
func (p *partitionRepository) validateCreate(ctx context.Context, req *adminv2.PartitionServiceCreateRequest) error {
	return validatePartition(ctx, req.Partition)
}

// ValidateDelete implements Partition.
func (p *partitionRepository) validateDelete(ctx context.Context, req *metal.Partition) error {
	// FIXME all entities with partition relation must be deleted before
	return nil
}

// ValidateUpdate implements Partition.
func (p *partitionRepository) validateUpdate(ctx context.Context, req *adminv2.PartitionServiceUpdateRequest, _ *metal.Partition) error {
	return validatePartition(ctx, req.Partition)
}

// Create implements Partition.
func (p *partitionRepository) create(ctx context.Context, c *adminv2.PartitionServiceCreateRequest) (*metal.Partition, error) {
	partition, err := p.convertToInternal(c.Partition)
	if err != nil {
		return nil, err
	}

	resp, err := p.r.ds.Partition().Create(ctx, partition)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// Delete implements Partition.
func (p *partitionRepository) delete(ctx context.Context, e *metal.Partition) error {
	err := p.r.ds.Partition().Delete(ctx, e)
	if err != nil {
		return err
	}

	return nil
}

// Get implements Partition.
func (p *partitionRepository) get(ctx context.Context, id string) (*metal.Partition, error) {
	partition, err := p.r.ds.Partition().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return partition, nil
}

// Update implements Partition.
func (p *partitionRepository) update(ctx context.Context, e *metal.Partition, req *adminv2.PartitionServiceUpdateRequest) (*metal.Partition, error) {
	partition := req.Partition

	new, err := p.convertToInternal(partition)
	if err != nil {
		return nil, err
	}

	new.SetChanged(e.Changed)

	err = p.r.ds.Partition().Update(ctx, new)
	if err != nil {
		return nil, err
	}
	return new, nil
}

// Find implements Partition.
func (p *partitionRepository) find(ctx context.Context, query *apiv2.PartitionQuery) (*metal.Partition, error) {
	partition, err := p.r.ds.Partition().Find(ctx, queries.PartitionFilter(query))
	if err != nil {
		return nil, err
	}
	return partition, nil
}

// List implements Partition.
func (p *partitionRepository) list(ctx context.Context, query *apiv2.PartitionQuery) ([]*metal.Partition, error) {
	partitions, err := p.r.ds.Partition().List(ctx, queries.PartitionFilter(query))
	if err != nil {
		return nil, err
	}
	return partitions, nil
}

// MatchScope implements Partition.
func (p *partitionRepository) matchScope(e *metal.Partition) bool {
	// Not Project Scoped
	return true
}

// ConvertToInternal implements Partition.
func (p *partitionRepository) convertToInternal(msg *apiv2.Partition) (*metal.Partition, error) {
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
func (p *partitionRepository) convertToProto(e *metal.Partition) (*apiv2.Partition, error) {
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
