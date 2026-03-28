package repository

import (
	"context"
	"slices"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"golang.org/x/crypto/ssh"
)

func (r *machineRepository) validateCreate(ctx context.Context, req *apiv2.MachineServiceCreateRequest) error {
	r.s.log.Debug("validate create", "req", req)
	project, err := r.s.Project(req.Project).Get(ctx, req.Project)
	if err != nil {
		return err
	}

	// TODO add Test, requires adoption in datacenter.go to create projects with quota
	// also add quota check for ipaddresses
	quotas, err := r.s.Project(req.Project).AdditionalMethods().GetQuotas(ctx, project.Uuid)
	if err != nil {
		return err
	}

	// Check if more machine would be allocated than project quota permits
	if quotas != nil && quotas.GetMachine() != nil {
		actualMachines, err := r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{
			Allocation: &apiv2.MachineAllocationQuery{
				Project: &req.Project,
				// TODO in metal-api this was set to FirewallRole ?
				AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE.Enum(),
			},
		}))
		if err != nil {
			return err
		}
		mq := quotas.GetMachine()
		if mq.Max != nil && len(actualMachines) >= int(*mq.Max) {
			return errorutil.FailedPrecondition("project quota for machines reached max:%d", *mq.Max)
		}
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
			return errorutil.NewInvalidArgument(err)
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

	// what do we have to prevent:
	// - user wants to place his machine in a network that does not belong to the project in which the machine is being placed
	// - user wants a machine with a private network that is not in the partition of the machine
	// - user specifies no private network
	// - user specifies multiple, unshared private networks
	// - user specifies a shared private network in addition to an unshared one for a machine
	// - user specifies administrative networks, i.e. underlay or privatesuper networks
	// - user's private network is specified with noauto but no specific IPs are given: this would yield a machine with no ip address

	var (
		networks                = map[string]bool{}
		childNetworkCount       int
		childSharedNetworkCount int
		externalNetworkCount    int
		underlayNetworkCount    int
		superNetworkCount       int
	)
	for _, nw := range req.Networks {
		n, err := r.s.UnscopedNetwork().Get(ctx, nw.Network)
		if err != nil {
			return err
		}
		if n.Project != nil && project.Uuid != *n.Project {
			if n.Type != nil && *n.Type != apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED {
				return errorutil.InvalidArgument("given network %s is project scoped but not part of project %s", nw.Network, req.Project)
			}
		}

		if n.Partition != nil && *n.Partition != partitionId {
			return errorutil.InvalidArgument("network %q must be located in the partition where the machine is going to be placed", n.Id)
		}

		if nw.NoAutoAcquireIp && len(nw.Ips) == 0 {
			return errorutil.InvalidArgument("the network %s has no auto ip acquisition, but no suitable IPs were provided, which would lead into a machine having no ip address", n.Id)
		}

		if n.Type != nil {
			if *n.Type == apiv2.NetworkType_NETWORK_TYPE_CHILD {
				childNetworkCount++
			}

			if *n.Type == apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED {
				childSharedNetworkCount++
			}

			if *n.Type == apiv2.NetworkType_NETWORK_TYPE_EXTERNAL {
				externalNetworkCount++
			}

			if *n.Type == apiv2.NetworkType_NETWORK_TYPE_UNDERLAY {
				underlayNetworkCount++
			}

			if *n.Type == apiv2.NetworkType_NETWORK_TYPE_SUPER || *n.Type == apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED {
				superNetworkCount++
			}
		}

		for _, ip := range nw.Ips {
			namespacedIP := metal.CreateNamespacedIPAddress(n.Namespace, ip)
			metalIP, err := r.s.ds.IP().Get(ctx, namespacedIP)
			if err != nil {
				return err
			}

			if n.Id != metalIP.NetworkID {
				return errorutil.InvalidArgument("given ip %s is not in the given network %s, which is required", n.Id, metalIP.IPAddress)
			}

			if metalIP.ProjectID != req.Project {
				return errorutil.InvalidArgument("given ip %s is not in the allocation project", metalIP.IPAddress)
			}

			// TODO explain this condition
			scope := metalIP.GetScope()
			if scope != metal.ScopeMachine && scope != metal.ScopeProject {
				return errorutil.InvalidArgument("given ip %s is not available for direct attachment to machine because it is already in use", metalIP.IPAddress)
			}
		}

		networks[n.Id] = true
	}

	if len(req.Networks) == 0 {
		return errorutil.InvalidArgument("networks must not be empty")
	}

	if len(networks) != len(req.Networks) {
		return errorutil.InvalidArgument("given network ids are not unique")
	}

	if underlayNetworkCount > 0 {
		return errorutil.InvalidArgument("firewalls must be allocated in a underlay but this must not be specified")
	}

	if superNetworkCount > 0 {
		return errorutil.InvalidArgument("super networks can not be specified as allocation networks")
	}

	if externalNetworkCount < 1 && req.AllocationType == apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL {
		return errorutil.InvalidArgument("firewalls must be allocated in at least one external network")
	}

	if (childNetworkCount+childSharedNetworkCount != 1) && req.AllocationType == apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE {
		return errorutil.InvalidArgument("machines must be allocated in exactly one child or child_shared network")
	}

	if childSharedNetworkCount > 1 {
		return errorutil.InvalidArgument("machines or firewalls must not be allocated in more than one child_shared network")
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
