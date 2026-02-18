package repository

import (
	"context"
	"slices"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

func (r *machineRepository) validateCreate(ctx context.Context, req *apiv2.MachineServiceCreateRequest) error {
	project, err := r.s.Project(req.Project).Get(ctx, req.Project)
	if err != nil {
		return err
	}

	var (
		partition *apiv2.Partition
		size      *apiv2.Size
		machine   *apiv2.Machine
	)

	if req.Uuid != nil {
		if req.Partition != "" {
			return errorutil.InvalidArgument("when no machine id is given, a partition must be specified")
		}
		if req.Size != "" {
			return errorutil.InvalidArgument("when no machine id is given, a size must be specified")
		}
		m, err := r.s.UnscopedMachine().Get(ctx, *req.Uuid)
		if err != nil {
			return err
		}
		if m.Allocation != nil {
			return errorutil.InvalidArgument("machine %s is already allocated", req.Uuid)
		}

		// TODO check if machine is waiting
		// TODO check if machine is locked or reserved

		machine = m

		partition = m.Partition
	} else {
		p, err := r.s.Partition().Get(ctx, req.Partition)
		if err != nil {
			return err
		}
		partition = p
		s, err := r.s.Size().Get(ctx, req.Size)
		if err != nil {
			return err
		}
		size = s
	}

	image, err := r.s.Image().Get(ctx, req.Image)
	if err != nil {
		return err
	}

	if req.FilesystemLayout != nil {
		fsl, err := r.s.FilesystemLayout().Get(ctx, *req.FilesystemLayout)
		if err != nil {
			return err
		}
		// Check if given fsl matches size and image
	} else {
		// fetch all fsl and search for a match
	}

	switch req.AllocationType {
	case apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL:
		if !slices.Contains(image.Features, apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL) {
			return errorutil.InvalidArgument("given image %s is not allowed for firewalls", image.Id)
		}
	case apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE:
		if !slices.Contains(image.Features, apiv2.ImageFeature_IMAGE_FEATURE_MACHINE) {
			return errorutil.InvalidArgument("given image %s is not allowed for machines", image.Id)
		}
		if req.FirewallSpec != nil {
			return errorutil.InvalidArgument("firewall rules can only be specified on firewalls", image.Id)
		}
	default:
		return errorutil.InvalidArgument("given allocationtype %s is not supported", req.AllocationType)
	}

	if len(req.Networks) == 0 {
		return errorutil.InvalidArgument("networks must not be empty")
	}

	var networks []string
	for _, nw := range req.Networks {
		n, err := r.s.UnscopedNetwork().Get(ctx, nw.Network)
		if err != nil {
			return err
		}
		if n.Project != nil && project.Uuid != *n.Project {
			return errorutil.InvalidArgument("given network %s is project scoped but not part of project %s", nw.Network, req.Project)
		}

		if n.Partition != nil && partition != nil && *n.Partition != partition.Id {
			return errorutil.InvalidArgument("network %q must be located in the partition where the machine is going to be placed", n.Id)
		}

		if n.Type != nil && *n.Type == apiv2.NetworkType_NETWORK_TYPE_CHILD {
			if nw.NoAutoAcquireIp != nil && !*nw.NoAutoAcquireIp && len(req.Ips) == 0 {
				return errorutil.InvalidArgument("the network %s has no auto ip acquisition, but no suitable IPs were provided, which would lead into a machine having no ip address", n.Id)
			}
		}

		networks = append(networks, nw.Network)
	}

	for _, ip := range req.Ips {
		// TODO namespaced ips
		i, err := r.s.IP(req.Project).Get(ctx, ip)
		if err != nil {
			return err
		}
		if !slices.Contains(networks, i.Network) {
			return errorutil.InvalidArgument("given ip %s is not in any of the given networks, which is required", i.Ip)
		}
		metalIP, err := r.s.ds.IP().Get(ctx, i.Ip)
		if err != nil {
			return err
		}
		scope := metalIP.GetScope()
		if scope != metal.ScopeMachine && scope != metal.ScopeProject {
			return errorutil.InvalidArgument("given ip %s is not available for direct attachment to machine because it is already in use", i.Ip)
		}

	}
	return nil
}

func (r *machineRepository) validateUpdate(ctx context.Context, req *apiv2.MachineServiceUpdateRequest, _ *metal.Machine) error {
	// FIXME implement with admin machine update
	return nil
}

func (r *machineRepository) validateDelete(ctx context.Context, req *metal.Machine) error {
	panic("unimplemented")

}
