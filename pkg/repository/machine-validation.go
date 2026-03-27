package repository

import (
	"context"
	"slices"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"golang.org/x/crypto/ssh"
)

func (r *machineRepository) validateCreate(ctx context.Context, req *apiv2.MachineServiceCreateRequest) error {
	project, err := r.s.Project(req.Project).Get(ctx, req.Project)
	if err != nil {
		return err
	}

	var (
		partitionId string
		size        *apiv2.Size
		machine     *metal.Machine
	)

	if req.Uuid != nil {
		if req.Partition != "" {
			return errorutil.InvalidArgument("when machine id is given, a partition must not be specified")
		}
		if req.Size != "" {
			return errorutil.InvalidArgument("when machine id is given, a size must not be specified")
		}

		m, err := r.s.ds.Machine().Get(ctx, *req.Uuid)
		if err != nil {
			return err
		}
		if m.Allocation != nil {
			return errorutil.InvalidArgument("machine %s is already allocated", *req.Uuid)
		}
		switch m.State.Value {
		case metal.LockedState, metal.ReservedState:
			return errorutil.InvalidArgument("machine %s is %s", *req.Uuid, m.State.Value)
		case metal.AvailableState:
			// Noop
		}
		if !m.Waiting {
			return errorutil.InvalidArgument("machine %s is not waiting", *req.Uuid)
		}

		machine = m
		partitionId = m.PartitionID
	} else {
		p, err := r.s.Partition().Get(ctx, req.Partition)
		if err != nil {
			return err
		}
		partitionId = p.Id
		s, err := r.s.Size().Get(ctx, req.Size)
		if err != nil {
			return err
		}
		size = s
	}

	images, err := r.s.Image().List(ctx, &apiv2.ImageQuery{})
	if err != nil {
		return err
	}
	image, err := r.s.Image().AdditionalMethods().GetMostRecentImageFor(ctx, req.Image, images)
	if err != nil {
		return err
	}

	if req.FilesystemLayout != nil {
		fsl, err := r.s.ds.FilesystemLayout().Get(ctx, *req.FilesystemLayout)
		if err != nil {
			return err
		}
		if machine != nil {
			// TODO this check must be done once a machine was selected
			if err := fsl.Matches(machine.Hardware); err != nil {
				return err
			}
		}
	}
	if req.FilesystemLayout == nil {
		var fsls metal.FilesystemLayouts
		fsls, err := r.s.ds.FilesystemLayout().List(ctx, nil)
		if err != nil {
			return err
		}
		_, err = fsls.From(size.Id, image.Id)
		if err != nil {
			return err
		}
	}

	switch req.AllocationType {
	case apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL:
		if !slices.Contains(image.Features, apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL) {
			return errorutil.InvalidArgument("given image %s is not allowed for firewalls", image.Id)
		}
		if err := validateFirewallSpec(req.FirewallSpec); err != nil {
			return err
		}
	case apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE:
		if !slices.Contains(image.Features, apiv2.ImageFeature_IMAGE_FEATURE_MACHINE) {
			return errorutil.InvalidArgument("given image %s is not allowed for machines", image.Id)
		}
		if req.FirewallSpec != nil {
			return errorutil.InvalidArgument("firewall rules can only be specified on firewalls")
		}
	default:
		return errorutil.InvalidArgument("given allocationtype %s is not supported", req.AllocationType)
	}

	if len(req.Networks) == 0 {
		return errorutil.InvalidArgument("networks must not be empty")
	}

	// TODO validate exactly one child or one child_shared (this is the case if this is the storage firewall and node) network for machines and firewalls
	// TODO validate zero or more child shared networks for firewalls
	// TODO validate at least one external network for firewalls

	var networks []string
	for _, nw := range req.Networks {
		// TODO external network is required
		n, err := r.s.UnscopedNetwork().Get(ctx, nw.Network)
		if err != nil {
			return err
		}
		if n.Project != nil && project.Uuid != *n.Project {
			return errorutil.InvalidArgument("given network %s is project scoped but not part of project %s", nw.Network, req.Project)
		}

		if n.Partition != nil && *n.Partition != partitionId {
			return errorutil.InvalidArgument("network %q must be located in the partition where the machine is going to be placed", n.Id)
		}

		if !nw.NoAutoAcquireIp && len(req.Ips) == 0 {
			return errorutil.InvalidArgument("the network %s has no auto ip acquisition, but no suitable IPs were provided, which would lead into a machine having no ip address", n.Id)
		}

		networks = append(networks, nw.Network)
	}

	for _, ip := range req.Ips {
		namespacedIP := metal.CreateNamespacedIPAddress(ip.Namespace, ip.Ip)
		metalIP, err := r.s.ds.IP().Get(ctx, namespacedIP)
		if err != nil {
			return err
		}
		// TODO Ip.Project == project
		if !slices.Contains(networks, metalIP.NetworkID) {
			return errorutil.InvalidArgument("given ip %s is not in any of the given networks, which is required", metalIP.IPAddress)
		}

		// TODO explain this condition
		scope := metalIP.GetScope()
		if scope != metal.ScopeMachine && scope != metal.ScopeProject {
			return errorutil.InvalidArgument("given ip %s is not available for direct attachment to machine because it is already in use", metalIP.IPAddress)
		}

	}

	for _, pubKey := range req.SshPublicKeys {
		_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKey))
		if err != nil {
			return errorutil.InvalidArgument("invalid public SSH key: %s error:%w", pubKey, err)
		}
	}

	return nil
}

func validateFirewallSpec(firewallSpec *apiv2.FirewallSpec) error {
	if firewallSpec == nil || firewallSpec.FirewallRules == nil {
		return nil
	}

	for _, rule := range firewallSpec.FirewallRules.Egress {
		protoString, err := enum.GetStringValue(rule.Protocol)
		if err != nil {
			return err
		}
		protocol, err := metal.ProtocolFromString(*protoString)
		if err != nil {
			return err
		}

		var ports []int
		for _, port := range rule.Ports {
			ports = append(ports, int(port))
		}

		metalEgress := metal.EgressRule{
			Protocol: protocol,
			Ports:    ports,
			To:       rule.To,
			Comment:  rule.Comment,
		}
		err = metalEgress.Validate()
		if err != nil {
			return err
		}
	}

	for _, rule := range firewallSpec.FirewallRules.Ingress {
		protoString, err := enum.GetStringValue(rule.Protocol)
		if err != nil {
			return err
		}
		protocol, err := metal.ProtocolFromString(*protoString)
		if err != nil {
			return err
		}

		var ports []int
		for _, port := range rule.Ports {
			ports = append(ports, int(port))
		}

		metalIngress := metal.IngressRule{
			Protocol: protocol,
			Ports:    ports,
			To:       rule.To,
			From:     rule.From,
			Comment:  rule.Comment,
		}
		err = metalIngress.Validate()
		if err != nil {
			return err
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
