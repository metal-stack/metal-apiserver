package repository

import (
	"context"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"golang.org/x/crypto/ssh"
)

func (r *machineRepository) validateCreate(ctx context.Context, req *apiv2.MachineServiceCreateRequest) error {
	r.s.log.Debug("validate create", "req", req)

	token, ok := token.TokenFromContext(ctx)
	if !ok {
		return errorutil.Unauthenticated("unable to get user from context")
	}

	project, err := r.s.Project(req.Project).Get(ctx, req.Project)
	if err != nil {
		return err
	}

	var (
		partitionId = pointer.SafeDeref(req.Partition)
		sizeId      = pointer.SafeDeref(req.Size)
		machine     *metal.Machine
	)

	if req.Uuid != nil {
		if token.AdminRole == nil || *token.AdminRole != apiv2.AdminRole_ADMIN_ROLE_EDITOR {
			return errorutil.Unauthenticated("only admins can create machines with a specific uuid")
		}

		if req.Partition != nil {
			return fmt.Errorf("when machine id is given, a partition must not be specified")
		}
		if req.Size != nil {
			return fmt.Errorf("when machine id is given, a size must not be specified")
		}

		m, err := r.s.ds.Machine().Get(ctx, *req.Uuid)
		if err != nil {
			return err
		}
		if m.Allocation != nil {
			return fmt.Errorf("machine %s is already allocated", *req.Uuid)
		}
		switch m.State.Value {
		case metal.LockedState:
			return fmt.Errorf("machine %s is %s", *req.Uuid, m.State.Value)
		case metal.AvailableState, metal.ReservedState:
			// machines which are reserved can be allocated by specifying the uuid,
			// but they will not be considered for random allocation
		}
		if !m.Waiting {
			return fmt.Errorf("machine %s is not waiting", *req.Uuid)
		}

		machine = m
		partitionId = m.PartitionID
		sizeId = m.SizeID
	}

	if _, err := r.s.ds.Partition().Get(ctx, partitionId); err != nil {
		return err
	}

	if _, err := r.s.ds.Size().Get(ctx, sizeId); err != nil {
		return err
	}

	image, err := r.s.Image().AdditionalMethods().GetMostRecentImageFor(ctx, &apiv2.ImageServiceLatestRequest{Os: req.Image})
	if err != nil {
		return err
	}

	if err := r.s.SizeImageConstraint().AdditionalMethods().Try(ctx, &apiv2.SizeImageConstraintServiceTryRequest{Size: sizeId, Image: image.Id}); err != nil {
		if !errorutil.IsNotFound(err) {
			return err
		}
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
		_, err = fsls.From(sizeId, image.Id)
		if err != nil {
			return err
		}
	}

	switch req.AllocationType {
	case apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL:
		if !slices.Contains(image.Features, apiv2.ImageFeature_IMAGE_FEATURE_FIREWALL) {
			return fmt.Errorf("given image %s is not allowed for firewalls", image.Id)
		}
		if err := r.validateFirewallSpec(req.FirewallSpec); err != nil {
			return err
		}
		underlay, err := r.s.ds.Network().Find(ctx, queries.NetworkFilter(&apiv2.NetworkQuery{
			Partition: &partitionId,
			Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum(),
		}))
		if err != nil {
			return err
		}
		if err := r.ipsAvailable(ctx, underlay.ID); err != nil {
			return err
		}

	case apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE:
		if !slices.Contains(image.Features, apiv2.ImageFeature_IMAGE_FEATURE_MACHINE) {
			return fmt.Errorf("given image %s is not allowed for machines", image.Id)
		}
		if req.FirewallSpec != nil {
			return fmt.Errorf("firewall rules can only be specified on firewalls")
		}
	default:
		return fmt.Errorf("given allocationtype %s is not supported", req.AllocationType)
	}

	// what do we have to prevent:
	// - user wants to place his machine in a network that does not belong to the project in which the machine is being placed
	// - user wants a machine with a private network that is not in the partition of the machine
	// - user specifies no private network
	// - user specifies multiple, unshared private networks
	// - user specifies a shared private network in addition to an unshared one for a machine
	// - user specifies administrative networks, i.e. underlay or super networks
	// - user's private network is specified with noauto but no specific IPs are given: this would yield a machine with no ip address

	var (
		networks         = map[string]bool{}
		networkTypeCount = make(map[apiv2.NetworkType]int)
	)
	for _, nw := range req.Networks {
		n, err := r.s.UnscopedNetwork().Get(ctx, nw.Network)
		if err != nil {
			return err
		}
		if n.Project != nil && project.Uuid != *n.Project {
			if n.Type != apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED {
				return fmt.Errorf("given network %s is project scoped but not part of project %s", nw.Network, req.Project)
			}
		}

		if n.Partition != nil && *n.Partition != partitionId {
			return fmt.Errorf("network %q must be located in the partition where the machine is going to be placed", n.Id)
		}

		networkTypeCount[n.Type]++

		if len(n.Prefixes) == 0 {
			return fmt.Errorf("network %q does not have any prefixes", n.Id)
		}

		if len(nw.Ips) == 0 {
			if err := r.ipsAvailable(ctx, nw.Network); err != nil {
				return err
			}
		}

		for _, ip := range nw.Ips {
			namespacedIP := metal.CreateNamespacedIPAddress(n.Namespace, ip)
			metalIP, err := r.s.ds.IP().Get(ctx, namespacedIP)
			if err != nil {
				return err
			}

			if n.Id != metalIP.NetworkID {
				return fmt.Errorf("given ip %s is not in the given network %s, which is required", n.Id, metalIP.IPAddress)
			}

			if metalIP.ProjectID != req.Project {
				return fmt.Errorf("given ip %s is not in the allocation project", metalIP.IPAddress)
			}

			scope := metalIP.GetScope()
			// Ensure that this ip is not already directly attached to another machine
			// TODO should we also ensure that this ip is not already used for a service type loadbalancer
			// TODO we also need to support anycast external ips in the metal only case.
			if scope == metal.ScopeMachine {
				return fmt.Errorf("given ip %s is not available for direct attachment to machine because it is already in use", metalIP.IPAddress)
			}
		}

		networks[n.Id] = true
	}

	if len(req.Networks) == 0 {
		return fmt.Errorf("networks must not be empty")
	}

	if len(networks) != len(req.Networks) {
		return fmt.Errorf("given network ids are not unique")
	}

	if networkTypeCount[apiv2.NetworkType_NETWORK_TYPE_UNDERLAY] > 0 {
		return fmt.Errorf("underlays cannot be specified in a machine allocation request (this is done automatically for firewalls)")
	}

	if (networkTypeCount[apiv2.NetworkType_NETWORK_TYPE_SUPER] + networkTypeCount[apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED]) > 0 {
		return fmt.Errorf("super networks can not be specified as allocation networks")
	}

	if networkTypeCount[apiv2.NetworkType_NETWORK_TYPE_EXTERNAL] == 0 && req.AllocationType == apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL {
		return fmt.Errorf("firewalls must be allocated in at least one external network")
	}

	if (networkTypeCount[apiv2.NetworkType_NETWORK_TYPE_CHILD]+networkTypeCount[apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED] != 1) && req.AllocationType == apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE {
		return fmt.Errorf("machines must be allocated in exactly one child or child_shared network")
	}

	if (networkTypeCount[apiv2.NetworkType_NETWORK_TYPE_CHILD]+networkTypeCount[apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED] < 1) && req.AllocationType == apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL {
		return fmt.Errorf("firewalls must have at least one child or child_shared network")
	}

	if networkTypeCount[apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED] > 1 {
		return fmt.Errorf("machines or firewalls must not be allocated in more than one child_shared network")
	}

	for _, pubKey := range req.SshPublicKeys {
		_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKey))
		if err != nil {
			return fmt.Errorf("invalid public SSH key: %s error:%w", pubKey, err)
		}
	}

	return nil
}

func (r *machineRepository) ipsAvailable(ctx context.Context, network string) error {
	metalnetwork, err := r.s.ds.Network().Get(ctx, network)
	if err != nil {
		return err
	}

	var availableIps uint64
	for _, pfx := range metalnetwork.Prefixes {
		usage, err := r.s.ipam.PrefixUsage(ctx, connect.NewRequest(&ipamv1.PrefixUsageRequest{
			Cidr:      pfx.String(),
			Namespace: metalnetwork.Namespace,
		}))
		if err != nil {
			return fmt.Errorf("unable to get network usage of: %s and prefixes:%s %w", network, pfx.String(), err)
		}
		availableIps += (usage.Msg.AvailableIps - usage.Msg.AcquiredIps)
	}
	r.s.log.Debug("ipsavailable", "network", network, "available", availableIps)
	if availableIps < 1 {
		return fmt.Errorf("no free ips in network %s", network)
	}
	return nil
}

func (r *machineRepository) validateFirewallSpec(firewallSpec *apiv2.FirewallSpec) error {
	if firewallSpec == nil || firewallSpec.FirewallRules == nil {
		return nil
	}

	_, err := r.convertFirewallRulesToInternal(firewallSpec.FirewallRules)
	return err
}

func (r *machineRepository) validateUpdate(ctx context.Context, req *apiv2.MachineServiceUpdateRequest, machine *metal.Machine) error {
	if machine.Allocation == nil {
		return fmt.Errorf("only allocated machines can be updated")
	}

	for _, pubKey := range req.SshPublicKeys {
		_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKey))
		if err != nil {
			return fmt.Errorf("invalid public SSH key: %s error:%w", pubKey, err)
		}
	}
	return nil
}

func (r *machineRepository) validateDelete(ctx context.Context, machine *metal.Machine) error {
	if machine.Allocation == nil {
		return fmt.Errorf("only allocated machines can be deleted")
	}

	if machine.State.Value == metal.LockedState {
		return fmt.Errorf("machine is locked and cannot be freed")
	}
	return nil
}
