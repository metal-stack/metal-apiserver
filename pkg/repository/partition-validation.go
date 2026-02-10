package repository

import (
	"context"
	"errors"
	"net"
	"net/netip"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

func validatePartition(ctx context.Context, partition *apiv2.Partition) error {
	//FIXME use validate helper
	if partition.Id == "" {
		return errorutil.InvalidArgument("partition id must not be empty")
	}

	if partition.BootConfiguration != nil {
		if err := checkIfUrlExists(ctx, "partition imageurl of", partition.Id, partition.BootConfiguration.ImageUrl); err != nil {
			return errorutil.NewInvalidArgument(err)
		}
		if err := checkIfUrlExists(ctx, "partition kernelurl of", partition.Id, partition.BootConfiguration.KernelUrl); err != nil {
			return errorutil.NewInvalidArgument(err)
		}
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
func (p *partitionRepository) validateCreate(ctx context.Context, req *adminv2.PartitionServiceCreateRequest) error {
	return validatePartition(ctx, req.Partition)
}

// ValidateDelete implements Partition.
func (p *partitionRepository) validateDelete(ctx context.Context, req *metal.Partition) error {
	var errs []error

	ms, err := p.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{Partition: &req.ID}))
	if err != nil {
		return err
	}

	p.s.log.Info("machines in partition", "partition", req.ID, "machines", ms)

	errs = validate(errs, len(ms) == 0, "there are still machines in %q", req.ID)

	nwsresp, err := p.s.ds.Network().List(ctx, queries.NetworkFilter(&apiv2.NetworkQuery{Partition: &req.ID}))
	if err != nil {
		return err
	}

	p.s.log.Info("networks in partition", "partition", req.ID, "networks", nwsresp)

	errs = validate(errs, len(nwsresp) == 0, "there are still networks in %q", req.ID)

	sizeReservations, err := p.s.ds.SizeReservation().List(ctx, queries.SizeReservationFilter(&apiv2.SizeReservationQuery{
		Partition: &req.ID,
	}))
	if err != nil {
		return err
	}
	errs = validate(errs, len(sizeReservations) == 0, "there are still size reservations in %q", req.ID)

	if err := errors.Join(errs...); err != nil {
		return errorutil.InvalidArgument("%w", err)
	}

	return nil
}
