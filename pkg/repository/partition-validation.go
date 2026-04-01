package repository

import (
	"context"
	"errors"
	"fmt"
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

func validatePartition(ctx context.Context, partition *apiv2.Partition) error {
	//FIXME use validate helper
	if partition.Id == "" {
		return fmt.Errorf("partition id must not be empty")
	}

	if partition.BootConfiguration != nil {
		if err := checkIfUrlExists(ctx, "partition imageurl of", partition.Id, partition.BootConfiguration.ImageUrl); err != nil {
			return err
		}
		if err := checkIfUrlExists(ctx, "partition kernelurl of", partition.Id, partition.BootConfiguration.KernelUrl); err != nil {
			return err
		}
	}

	if len(partition.DnsServers) > 3 {
		return fmt.Errorf("not more than 3 dnsservers must be specified")
	}

	for _, dns := range partition.DnsServers {
		_, err := netip.ParseAddr(dns.Ip)
		if err != nil {
			return fmt.Errorf("dnsserver ip is not valid:%w", err)
		}
	}

	if len(partition.NtpServers) > 5 {
		return fmt.Errorf("not more than 5 ntpservers must be specified")
	}

	for _, ntp := range partition.NtpServers {
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
	var errs []error

	ms, err := p.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{Partition: &req.ID}))
	if err != nil {
		return errorutil.NewInternal(err)
	}

	errs = validate(errs, len(ms) == 0, "there are still machines in %q", req.ID)

	nwsresp, err := p.s.ds.Network().List(ctx, queries.NetworkFilter(&apiv2.NetworkQuery{Partition: &req.ID}))
	if err != nil {
		return errorutil.NewInternal(err)
	}

	errs = validate(errs, len(nwsresp) == 0, "there are still networks in %q", req.ID)

	sizeReservations, err := p.s.ds.SizeReservation().List(ctx, queries.SizeReservationFilter(&apiv2.SizeReservationQuery{
		Partition: &req.ID,
	}))
	if err != nil {
		return errorutil.NewInternal(err)
	}

	errs = validate(errs, len(sizeReservations) == 0, "there are still size reservations in %q", req.ID)

	if err := errors.Join(errs...); err != nil {
		return err
	}

	return nil
}

// ValidateUpdate implements Partition.
func (p *partitionRepository) validateUpdate(ctx context.Context, req *adminv2.PartitionServiceUpdateRequest, _ *metal.Partition) error {
	partition := &apiv2.Partition{
		Id:                   req.Id,
		BootConfiguration:    req.BootConfiguration,
		DnsServers:           req.DnsServers,
		NtpServers:           req.NtpServers,
		MgmtServiceAddresses: req.MgmtServiceAddresses,
	}
	return validatePartition(ctx, partition)
}
