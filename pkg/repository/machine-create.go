package repository

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	v1 "github.com/metal-stack/masterdata-api/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/tag"
	"golang.org/x/crypto/ssh"
)

// machineAllocationSpec is a specification for a machine allocation
type machineAllocationSpec struct {
	Creator            string
	UUID               string
	Name               string
	Description        string
	Hostname           string
	ProjectID          string
	PartitionID        string
	Machine            *metal.Machine
	Size               *metal.Size
	Image              *metal.Image
	FilesystemLayoutID *string
	SSHPubKeys         []string
	UserData           string
	Tags               []string
	Networks           []*apiv2.MachineAllocationNetwork
	IPs                []*apiv2.MachineAllocationIp
	Role               metal.Role
	VPN                *metal.MachineVPN
	PlacementTags      []string
	EgressRules        []metal.EgressRule
	IngressRules       []metal.IngressRule
	DNSServers         metal.DNSServers
	NTPServers         metal.NTPServers
}

// allocationNetwork is intermediate struct to create machine networks from regular networks during machine allocation
type allocationNetwork struct {
	network *metal.Network
	ips     []*metal.IP
	auto    bool

	networkType metal.NetworkType
}

func (r *machineRepository) createMachineAllocationSpec(ctx context.Context, req *apiv2.MachineServiceCreateRequest) (*machineAllocationSpec, error) {
	var uuid string
	if req.Uuid != nil {
		uuid = *req.Uuid
	}
	name := req.Name
	var description string
	if req.Description != nil {
		description = *req.Description
	}
	hostname := "metal"
	if req.Hostname != nil {
		hostname = *req.Hostname
	}
	var userdata string
	if req.Userdata != nil {
		userdata = *req.Userdata
	}

	image, err := r.s.ds.Image().Get(ctx, req.Image)
	if err != nil {
		return nil, err
	}

	var (
		egress  []metal.EgressRule
		ingress []metal.IngressRule
		role    = metal.RoleMachine
	)

	if req.AllocationType == apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL {
		role = metal.RoleFirewall

		if req.FirewallSpec != nil && req.FirewallSpec.FirewallRules != nil {
			for _, ruleSpec := range req.FirewallSpec.FirewallRules.Egress {
				protocolLowerCase, err := enum.GetStringValue(ruleSpec.Protocol)
				if err != nil {
					return nil, err
				}
				protocol, err := metal.ProtocolFromString(*protocolLowerCase)
				if err != nil {
					return nil, err
				}

				var ports []int
				for _, p := range ruleSpec.Ports {
					ports = append(ports, int(p))
				}

				rule := metal.EgressRule{
					Protocol: protocol,
					Ports:    ports,
					To:       ruleSpec.To,
					Comment:  ruleSpec.Comment,
				}

				if err := rule.Validate(); err != nil {
					return nil, err
				}

				egress = append(egress, rule)
			}

			for _, ruleSpec := range req.FirewallSpec.FirewallRules.Ingress {
				protocolLowerCase, err := enum.GetStringValue(ruleSpec.Protocol)
				if err != nil {
					return nil, err
				}
				protocol, err := metal.ProtocolFromString(*protocolLowerCase)
				if err != nil {
					return nil, err
				}

				var ports []int
				for _, p := range ruleSpec.Ports {
					ports = append(ports, int(p))
				}

				rule := metal.IngressRule{
					Protocol: protocol,
					Ports:    ports,
					To:       ruleSpec.To,
					From:     ruleSpec.From,
					Comment:  ruleSpec.Comment,
				}

				if err := rule.Validate(); err != nil {
					return nil, err
				}

				ingress = append(ingress, rule)
			}
		}
	}

	partitionID := req.Partition
	sizeID := req.Size

	if uuid == "" && partitionID == "" {
		return nil, errors.New("when no machine id is given, a partition id must be specified")
	}

	if uuid == "" && sizeID == "" {
		return nil, errors.New("when no machine id is given, a size id must be specified")
	}

	var m *metal.Machine
	// Allocation of a specific machine is requested, therefore size and partition are not given, fetch them
	if uuid != "" {
		m, err = r.s.ds.Machine().Get(ctx, uuid)
		if err != nil {
			return nil, fmt.Errorf("uuid given but no machine found with uuid:%s err:%w", uuid, err)
		}
		sizeID = m.SizeID
		partitionID = m.PartitionID
	}

	size, err := r.s.ds.Size().Get(ctx, sizeID)
	if err != nil {
		return nil, fmt.Errorf("size:%s not found err:%w", sizeID, err)
	}

	partition, err := r.s.ds.Partition().Get(ctx, partitionID)
	if err != nil {
		return nil, fmt.Errorf("partition:%s not found err:%w", partitionID, err)
	}

	var (
		dnsServers = partition.DNSServers
		ntpServers = partition.NTPServers
	)
	if len(req.DnsServer) != 0 {
		dnsServers = metal.DNSServers{}
		for _, s := range req.DnsServer {
			dnsServers = append(dnsServers, metal.DNSServer{
				IP: s.Ip,
			})
		}
	}
	if len(req.NtpServer) != 0 {
		ntpServers = []metal.NTPServer{}
		for _, s := range req.NtpServer {
			ntpServers = append(ntpServers, metal.NTPServer{
				Address: s.Address,
			})
		}
	}

	var tags []string
	if req.Labels != nil {
		for k, v := range req.Labels.Labels {
			tags = append(tags, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// figure out creator
	var creator string
	token, ok := token.TokenFromContext(ctx)
	if ok {
		creator = token.User
	}

	return &machineAllocationSpec{
		Creator:            creator,
		UUID:               uuid,
		Name:               name,
		Description:        description,
		Hostname:           hostname,
		ProjectID:          req.Project,
		PartitionID:        partitionID,
		Machine:            m,
		Size:               size,
		Image:              image,
		SSHPubKeys:         req.SshPublicKeys,
		UserData:           userdata,
		Tags:               tags,
		Networks:           req.Networks,
		IPs:                req.Ips,
		Role:               role,
		FilesystemLayoutID: req.FilesystemLayout,
		PlacementTags:      req.PlacementTags,
		EgressRules:        egress,
		IngressRules:       ingress,
		DNSServers:         dnsServers,
		NTPServers:         ntpServers,
	}, nil

}

func (r *machineRepository) allocateMachine(ctx context.Context, spec *machineAllocationSpec) (*metal.Machine, error) {
	if err := r.validateAllocationSpec(spec); err != nil {
		return nil, err
	}

	if err := r.isSizeAndImageCompatible(ctx, spec.Size, spec.Image); err != nil {
		return nil, err
	}

	p, err := r.s.mdc.Project().Get(ctx, &v1.ProjectGetRequest{Id: spec.ProjectID})
	if err != nil {
		return nil, err
	}

	// Check if more machine would be allocated than project quota permits
	if p.GetProject() != nil && p.GetProject().GetQuotas() != nil && p.GetProject().GetQuotas().GetMachine() != nil {
		mq := p.GetProject().GetQuotas().GetMachine()
		maxMachines := mq.Max
		actualMachines, err := r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{
			Allocation: &apiv2.MachineAllocationQuery{
				Project: &spec.ProjectID,
				// TODO in metal-api this was set to FirewallRole ?
				AllocationType: apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_MACHINE.Enum(),
			},
		}))
		if err != nil {
			return nil, err
		}
		if maxMachines != nil && len(actualMachines) >= int(*maxMachines) {
			return nil, fmt.Errorf("project quota for machines reached max:%d", *maxMachines)
		}
	}

	var fsl *metal.FilesystemLayout
	if spec.FilesystemLayoutID == nil {
		var fsls metal.FilesystemLayouts
		fsls, err := r.s.ds.FilesystemLayout().List(ctx, nil)
		if err != nil {
			return nil, err
		}
		_, err = fsls.From(spec.Size.ID, spec.Image.ID)
		if err != nil {
			return nil, err
		}
	} else {
		fsl, err = r.s.ds.FilesystemLayout().Get(ctx, *spec.FilesystemLayoutID)
		if err != nil {
			return nil, err
		}
	}

	machineCandidate, err := r.findMachineCandidate(ctx, spec)
	if err != nil {
		return nil, err
	}

	var firewallRules *metal.FirewallRules
	if len(spec.EgressRules) > 0 || len(spec.IngressRules) > 0 {
		firewallRules = &metal.FirewallRules{
			Egress:  spec.EgressRules,
			Ingress: spec.IngressRules,
		}
	}

	// as some fields in the allocation spec are optional, they will now be clearly defined by the machine candidate
	spec.UUID = machineCandidate.ID

	alloc := &metal.MachineAllocation{
		Creator:         spec.Creator,
		Created:         time.Now(),
		Name:            spec.Name,
		Description:     spec.Description,
		Hostname:        spec.Hostname,
		Project:         spec.ProjectID,
		ImageID:         spec.Image.ID,
		UserData:        spec.UserData,
		SSHPubKeys:      spec.SSHPubKeys,
		MachineNetworks: []*metal.MachineNetwork{},
		Role:            spec.Role,
		VPN:             spec.VPN,
		FirewallRules:   firewallRules,
		UUID:            uuid.New().String(),
		DNSServers:      spec.DNSServers,
		NTPServers:      spec.NTPServers,
	}

	// TODO this must be done in a rollbackTask maybe triggered from the caller
	// rollbackOnError := func(err error) error {
	// 	if err != nil {
	// 		cleanupMachine := &metal.Machine{
	// 			Base: metal.Base{
	// 				ID: spec.UUID,
	// 			},
	// 			Allocation: alloc,
	// 		}
	// 		rollbackError := actor.machineNetworkReleaser(cleanupMachine)
	// 		if rollbackError != nil {
	// 			logger.Error("cannot call async machine cleanup", "error", rollbackError)
	// 		}
	// 		old := *machineCandidate
	// 		machineCandidate.Allocation = nil
	// 		machineCandidate.Tags = nil
	// 		machineCandidate.PreAllocated = false

	// 		rollbackError = ds.UpdateMachine(&old, machineCandidate)
	// 		if rollbackError != nil {
	// 			logger.Error("cannot update machinecandidate to reset allocation", "error", rollbackError)
	// 		}
	// 	}
	// 	return err
	// }

	err = fsl.Matches(machineCandidate.Hardware)
	if err != nil {
		return nil, rollbackOnError(fmt.Errorf("unable to check for fsl match:%w", err))
	}
	alloc.FilesystemLayout = fsl

	networks, err := r.gatherNetworks(ctx, spec)
	if err != nil {
		return nil, rollbackOnError(fmt.Errorf("unable to gather networks:%w", err))
	}
	err = r.makeNetworks(ctx, spec, networks, alloc)
	if err != nil {
		return nil, rollbackOnError(fmt.Errorf("unable to make networks:%w", err))
	}

	// refetch the machine to catch possible updates after dealing with the network...
	machine, err := r.s.ds.Machine().Get(ctx, machineCandidate.ID)
	if err != nil {
		return nil, rollbackOnError(fmt.Errorf("unable to find machine:%w", err))
	}
	if machine.Allocation != nil {
		return nil, rollbackOnError(fmt.Errorf("machine %q already allocated", machine.ID))
	}

	machine.Allocation = alloc
	machine.Tags = r.makeMachineTags(machine, spec.Tags)
	machine.PreAllocated = false

	err = r.s.ds.Machine().Update(ctx, machine)
	if err != nil {
		return nil, rollbackOnError(fmt.Errorf("error when allocating machine %q, %w", machine.ID, err))
	}

	// TODO: can be removed after metal-core refactoring
	// err = publisher.Publish(metal.TopicAllocation.Name, &metal.AllocationEvent{MachineID: machine.ID})
	// if err != nil {
	// 	logger.Error("failed to publish machine allocation event, fallback should trigger on metal-hammer", "topic", metal.TopicAllocation.Name, "machineID", machine.ID, "error", err)
	// } else {
	// 	logger.Debug("published machine allocation event", "topic", metal.TopicAllocation.Name, "machineID", machine.ID)
	// }

	return machine, nil

}

func (r *machineRepository) validateAllocationSpec(allocationSpec *machineAllocationSpec) error {
	if allocationSpec.ProjectID == "" {
		return errors.New("project id must be specified")
	}

	if allocationSpec.Creator == "" {
		return errors.New("creator should be specified")
	}

	if !metal.AllRoles[allocationSpec.Role] {
		return fmt.Errorf("role does not exist: %s", allocationSpec.Role)
	}

	for _, ip := range allocationSpec.IPs {
		if net.ParseIP(ip) == nil {
			return fmt.Errorf("%q is not a valid IP address", ip)
		}
	}

	for _, pubKey := range allocationSpec.SSHPubKeys {
		_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKey))
		if err != nil {
			return fmt.Errorf("invalid public SSH key: %s error:%w", pubKey, err)
		}
	}

	// A firewall must have either IP or Network with auto IP acquire specified.
	if allocationSpec.Role == metal.RoleFirewall {
		if len(allocationSpec.IPs) == 0 && allocationSpec.autoNetworkN() == 0 {
			return errors.New("when no ip is given at least one auto acquire network must be specified")
		}
	}

	if noautoNetN := allocationSpec.noautoNetworkN(); noautoNetN > len(allocationSpec.IPs) {
		return errors.New("missing ip(s) for network(s) without automatic ip allocation")
	}

	return nil
}

func (r *machineRepository) isSizeAndImageCompatible(ctx context.Context, size *metal.Size, image *metal.Image) error {
	sic, err := r.s.ds.FindSizeImageConstraint(size.ID)
	if err != nil && !metal.IsNotFound(err) {
		return err
	}
	if sic == nil {
		return nil
	}

	return sic.Matches(size, image)
}

func (r *machineRepository) findMachineCandidate(ctx context.Context, spec *machineAllocationSpec) (*metal.Machine, error) {
	var (
		err     error
		machine *metal.Machine
	)
	if spec.Machine == nil {
		// requesting allocation of an arbitrary ready machine in partition with given size
		machine, err = findWaitingMachine(ctx, ds, spec)
		if err != nil {
			return nil, err
		}
	} else {
		// requesting allocation of a specific, existing machine
		machine = spec.Machine
		if machine.Allocation != nil {
			return nil, errors.New("machine is already allocated")
		}
		if spec.PartitionID != "" && machine.PartitionID != spec.PartitionID {
			return nil, fmt.Errorf("machine %q is not in the requested partition: %s", machine.ID, allocationSpec.PartitionID)
		}

		if spec.Size != nil && machine.SizeID != spec.Size.ID {
			return nil, fmt.Errorf("machine %q does not have the requested size: %s", machine.ID, allocationSpec.Size.ID)
		}
	}
	return machine, err
}

func (r *machineRepository) findWaitingMachine(ctx context.Context, spec *machineAllocationSpec) (*metal.Machine, error) {
	size, err := ds.FindSize(spec.Size.ID)
	if err != nil {
		return nil, fmt.Errorf("size cannot be found: %w", err)
	}
	partition, err := ds.FindPartition(spec.PartitionID)
	if err != nil {
		return nil, fmt.Errorf("partition cannot be found: %w", err)
	}

	machine, err := ds.FindWaitingMachine(ctx, spec.ProjectID, partition.ID, *size, spec.PlacementTags, spec.Role)
	if err != nil {
		return nil, err
	}
	return machine, nil
}

// makeNetworks creates network entities and ip addresses as specified in the allocation network map.
// created networks are added to the machine allocation directly after their creation. This way, the rollback mechanism
// is enabled to clean up networks that were already created.
func (r *machineRepository) makeNetworks(ctx context.Context, spec *machineAllocationSpec, networks allocationNetworkMap, alloc *metal.MachineAllocation) error {
	for _, n := range networks {
		if n == nil || n.network == nil {
			continue
		}
		machineNetwork, err := r.makeMachineNetwork(ctx, spec, n)
		if err != nil {
			return err
		}
		alloc.MachineNetworks = append(alloc.MachineNetworks, machineNetwork)
	}

	// the metal-networker expects to have the same unique ASN on all networks of this machine
	asn, err := acquireASN(ds)
	if err != nil {
		return err
	}
	for _, n := range alloc.MachineNetworks {
		n.ASN = *asn
	}

	return nil
}

func (r *machineRepository) gatherNetworks(ctx context.Context, spec *machineAllocationSpec) (allocationNetworkMap, error) {
	partition, err := ds.FindPartition(spec.PartitionID)
	if err != nil {
		return nil, fmt.Errorf("partition cannot be found: %w", err)
	}

	var privateSuperNetworks metal.Networks
	boolTrue := true
	err = ds.SearchNetworks(&datastore.NetworkSearchQuery{PrivateSuper: &boolTrue}, &privateSuperNetworks)
	if err != nil {
		return nil, fmt.Errorf("partition has no private super network: %w", err)
	}

	specNetworks, err := gatherNetworksFromSpec(ds, spec, partition, privateSuperNetworks)
	if err != nil {
		return nil, err
	}

	var underlayNetwork *allocationNetwork
	if spec.Role == metal.RoleFirewall {
		underlayNetwork, err = gatherUnderlayNetwork(ds, partition)
		if err != nil {
			return nil, err
		}
	}

	// assemble result
	result := specNetworks
	if underlayNetwork != nil {
		result[underlayNetwork.network.ID] = underlayNetwork
	}

	return result, nil
}

func (r *machineRepository) gatherNetworksFromSpec(ctx context.Context, spec *machineAllocationSpec, partition *metal.Partition, privateSuperNetworks metal.Networks) (allocationNetworkMap, error) {
	var partitionPrivateSuperNetwork *metal.Network
	for i := range privateSuperNetworks {
		psn := privateSuperNetworks[i]
		if partition.ID == psn.PartitionID {
			partitionPrivateSuperNetwork = &psn
			break
		}
	}
	if partitionPrivateSuperNetwork == nil {
		return nil, fmt.Errorf("partition %s does not have a private super network", partition.ID)
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
		specNetworks          = make(map[string]*allocationNetwork)
		primaryPrivateNetwork *allocationNetwork
		privateNetworks       []*allocationNetwork
		privateSharedNetworks []*allocationNetwork
	)

	for _, networkSpec := range spec.Networks {
		auto := true
		if networkSpec.NoAutoAcquireIp != nil {
			auto = *networkSpec.NoAutoAcquireIp
		}

		network, err := ds.FindNetworkByID(networkSpec.Network)
		if err != nil {
			return nil, err
		}

		if network.Underlay {
			return nil, fmt.Errorf("underlay networks are not allowed to be set explicitly: %s", network.ID)
		}
		if network.PrivateSuper {
			return nil, fmt.Errorf("private super networks are not allowed to be set explicitly: %s", network.ID)
		}

		n := &allocationNetwork{
			network:     network,
			auto:        auto,
			ips:         []*metal.IP{},
			networkType: metal.External,
		}

		for _, privateSuperNetwork := range privateSuperNetworks {
			if network.ParentNetworkID != privateSuperNetwork.ID {
				continue
			}
			if network.Shared {
				n.networkType = metal.PrivateSecondaryShared
				privateSharedNetworks = append(privateSharedNetworks, n)
			} else {
				if primaryPrivateNetwork != nil {
					return nil, errors.New("multiple private networks are specified but there must be only one primary private network that must not be shared")
				}
				n.networkType = metal.PrivatePrimaryUnshared
				primaryPrivateNetwork = n
			}
			privateNetworks = append(privateNetworks, n)
		}

		specNetworks[network.ID] = n
	}

	if len(specNetworks) != len(spec.Networks) {
		return nil, errors.New("given network ids are not unique")
	}

	if len(privateNetworks) == 0 {
		return nil, errors.New("no private network given")
	}

	// if there is no unshared private network we try to determine a shared one as primary
	if primaryPrivateNetwork == nil {
		// this means that this is a machine of a shared private network
		// this is an exception where the primary private network is a shared one.
		// it must be the only private network
		if len(privateSharedNetworks) == 0 {
			return nil, errors.New("no private shared network found that could be used as primary private network")
		}
		if len(privateSharedNetworks) > 1 {
			return nil, errors.New("machines and firewalls are not allowed to be placed into multiple private, shared networks (firewall needs an unshared private network and machines may only reside in one private network)")
		}

		primaryPrivateNetwork = privateSharedNetworks[0]
		primaryPrivateNetwork.networkType = metal.PrivatePrimaryShared
	}

	if spec.Role == metal.RoleMachine && len(privateNetworks) > 1 {
		return nil, errors.New("machines are not allowed to be placed into multiple private networks")
	}

	if primaryPrivateNetwork.network.ProjectID != spec.ProjectID {
		return nil, errors.New("the given private network does not belong to the project, which is not allowed")
	}

	for _, ipString := range spec.IPs {
		ip, err := ds.FindIPByID(ipString)
		if err != nil {
			return nil, err
		}
		if ip.ProjectID != spec.ProjectID {
			return nil, fmt.Errorf("given ip %q with project id %q does not belong to the project of this allocation: %s", ip.IPAddress, ip.ProjectID, spec.ProjectID)
		}
		network, ok := specNetworks[ip.NetworkID]
		if !ok {
			return nil, fmt.Errorf("given ip %q is not in any of the given networks, which is required", ip.IPAddress)
		}
		s := ip.GetScope()
		if s != metal.ScopeMachine && s != metal.ScopeProject {
			return nil, fmt.Errorf("given ip %q is not available for direct attachment to machine because it is already in use", ip.IPAddress)
		}

		network.auto = false
		network.ips = append(network.ips, *ip)
	}

	for _, privateNetwork := range privateNetworks {
		if privateNetwork.network.PartitionID != partitionPrivateSuperNetwork.PartitionID {
			return nil, fmt.Errorf("private network %q must be located in the partition where the machine is going to be placed", privateNetwork.network.ID)
		}

		if !privateNetwork.auto && len(privateNetwork.ips) == 0 {
			return nil, fmt.Errorf("the private network %q has no auto ip acquisition, but no suitable IPs were provided, which would lead into a machine having no ip address", privateNetwork.network.ID)
		}
	}

	return specNetworks, nil
}

func (r *machineRepository) gatherUnderlayNetwork(ctx context.Context, partitionId string) (*allocationNetwork, error) {
	underlay, err := r.s.ds.Network().Find(ctx, queries.NetworkFilter(&apiv2.NetworkQuery{
		Partition: &partitionId,
		Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum(),
	}))
	if err != nil {
		return nil, err
	}

	return &allocationNetwork{
		network:     underlay,
		auto:        true,
		networkType: metal.Underlay,
	}, nil
}

func (r *machineRepository) makeMachineNetwork(ctx context.Context, spec *machineAllocationSpec, n *allocationNetwork) (*metal.MachineNetwork, error) {
	if n.auto {
		if len(n.network.Prefixes) == 0 {
			return nil, fmt.Errorf("given network %s does not have prefixes configured", n.network.ID)
		}
		for _, af := range n.network.Prefixes.AddressFamilies() {
			ipAddress, ipParentCidr, err := r.s.IP(spec.ProjectID).AdditionalMethods().allocateRandomIP(ctx, n.network, af)
			if err != nil {
				return nil, fmt.Errorf("unable to allocate an ip in network: %s %w", n.network.ID, err)
			}
			ip := &metal.IP{
				IPAddress:        ipAddress,
				ParentPrefixCidr: ipParentCidr,
				Name:             spec.Name,
				Description:      "autoassigned",
				NetworkID:        n.network.ID,
				Type:             metal.Ephemeral,
				ProjectID:        spec.ProjectID,
			}
			// FIXME ugly implementation
			ip.AddMachineId(spec.UUID)

			_, err = r.s.ds.IP().Create(ctx, ip)
			if err != nil {
				return nil, err
			}
			n.ips = append(n.ips, ip)
		}
	}

	// from the makeNetworks call, a lot of ips might be set in this network
	// add a machine tag to all of them
	var ipAddresses []string
	for i := range n.ips {
		ip := n.ips[i]
		newIP := ip
		newIP.AddMachineId(spec.UUID)
		err := r.s.ds.IP().Update(ctx, newIP)
		if err != nil {
			return nil, err
		}
		ipAddresses = append(ipAddresses, ip.IPAddress)
	}

	machineNetwork := metal.MachineNetwork{
		NetworkID:           n.network.ID,
		Prefixes:            n.network.Prefixes.String(),
		IPs:                 ipAddresses,
		DestinationPrefixes: n.network.DestinationPrefixes.String(),
		PrivatePrimary:      n.networkType.PrivatePrimary,
		Private:             n.networkType.Private,
		Shared:              n.networkType.Shared,
		Underlay:            n.networkType.Underlay,
		Nat:                 n.network.Nat,
		Vrf:                 n.network.Vrf,
	}

	return &machineNetwork, nil
}

// makeMachineTags constructs the tags of the machine.
// following tags are added in the following precedence (from lowest to highest in case of duplication):
// - user given tags (from allocation spec)
// - system tags (immutable information from the metal-api that are useful for the end user, e.g. machine rack and chassis)
func (r *machineRepository) makeMachineTags(m *metal.Machine, userTags []string) []string {
	labels := make(map[string]string)

	// as user labels are given as an array, we need to figure out if label-like tags were provided.
	// otherwise the user could provide confusing information like:
	// - machine.metal-stack.io/chassis=123
	// - machine.metal-stack.io/chassis=789
	userLabels := make(map[string]string)
	actualUserTags := []string{}
	for _, tag := range userTags {
		if strings.Contains(tag, "=") {
			parts := strings.SplitN(tag, "=", 2)
			userLabels[parts[0]] = parts[1]
		} else {
			actualUserTags = append(actualUserTags, tag)
		}
	}
	for k, v := range userLabels {
		labels[k] = v
	}

	for k, v := range r.makeMachineSystemLabels(m) {
		labels[k] = v
	}

	tags := actualUserTags
	for k, v := range labels {
		tags = append(tags, fmt.Sprintf("%s=%s", k, v))
	}

	return uniqueTags(tags)
}

func (r *machineRepository) makeMachineSystemLabels(m *metal.Machine) map[string]string {
	labels := make(map[string]string)
	for _, n := range m.Allocation.MachineNetworks {
		if n.Private {
			if n.ASN != 0 {
				labels[tag.MachineNetworkPrimaryASN] = strconv.FormatInt(int64(n.ASN), 10)
				break
			}
		}
	}
	if m.RackID != "" {
		labels[tag.MachineRack] = m.RackID
	}
	if m.IPMI.Fru.ChassisPartSerial != "" {
		labels[tag.MachineChassis] = m.IPMI.Fru.ChassisPartSerial
	}
	return labels
}

// uniqueTags the last added tags will be kept!
func uniqueTags(tags []string) []string {
	tagSet := make(map[string]bool)
	for _, t := range tags {
		tagSet[t] = true
	}
	uniqueTags := []string{}
	for k := range tagSet {
		uniqueTags = append(uniqueTags, k)
	}
	return uniqueTags
}
