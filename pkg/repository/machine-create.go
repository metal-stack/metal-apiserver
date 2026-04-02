package repository

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/metal-stack/metal-lib/pkg/tag"
	"github.com/samber/lo"
)

// machineAllocationSpec is a specification for a machine allocation
// FIXME this is weird
type machineAllocationSpec struct {
	UUID        string
	PartitionID string
	allocation  metal.MachineAllocation
	Machine     *metal.Machine
	Size        *metal.Size
	Image       *metal.Image
	// Tags          []string
	Networks      []*apiv2.MachineAllocationNetwork
	PlacementTags []string
}

// allocationNetwork is intermediate struct to create machine networks from regular networks during machine allocation
type allocationNetwork struct {
	network *metal.Network
	ips     []*metal.IP
	auto    bool
}

func (r *machineRepository) createMachineAllocationSpec(ctx context.Context, req *apiv2.MachineServiceCreateRequest) (*machineAllocationSpec, error) {
	var (
		uuid        = pointer.SafeDeref(req.Uuid)
		name        = req.Name
		description = pointer.SafeDeref(req.Description)
		hostname    = pointer.SafeDerefOrDefault(req.Hostname, "metal")
		userdata    = pointer.SafeDeref(req.Userdata)
		creator     string

		partitionID = pointer.SafeDeref(req.Partition)
		sizeID      = pointer.SafeDeref(req.Size)

		fwrules *metal.FirewallRules
		role    = metal.RoleMachine

		m *metal.Machine
	)

	// figure out creator
	token, ok := token.TokenFromContext(ctx)
	if ok {
		creator = token.User
	} else {
		// TODO can we ensure we get a token with the correct user if called from mcm ?
		// Or is it sufficient if the cluster creator is set correct.
		return nil, errorutil.Unauthenticated("unable to get user from context")
	}

	if req.AllocationType == apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL {
		role = metal.RoleFirewall
		if req.FirewallSpec != nil && req.FirewallSpec.FirewallRules != nil {
			fwr, err := r.convertFirewallRulesToInternal(req.FirewallSpec.FirewallRules)
			if err != nil {
				return nil, err
			}
			fwrules = fwr
		}
	}

	// Allocation of a specific machine is requested, therefore size and partition are not given, fetch them
	if uuid != "" {
		possibleMachine, err := r.s.ds.Machine().Get(ctx, uuid)
		if err != nil {
			return nil, fmt.Errorf("uuid given but no machine found with uuid:%s err:%w", uuid, err)
		}

		m = possibleMachine
		sizeID = m.SizeID
		partitionID = m.PartitionID
	}

	image, err := r.s.ds.Image().Get(ctx, req.Image)
	if err != nil {
		return nil, err
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

	// DNS and NTP Servers from request have precedence
	if len(req.DnsServers) != 0 {
		dnsServers = metal.DNSServers{}
		for _, s := range req.DnsServers {
			dnsServers = append(dnsServers, metal.DNSServer{
				IP: s.Ip,
			})
		}
	}
	if len(req.NtpServers) != 0 {
		ntpServers = []metal.NTPServer{}
		for _, s := range req.NtpServers {
			ntpServers = append(ntpServers, metal.NTPServer{
				Address: s.Address,
			})
		}
	}

	var (
		labels = make(map[string]string)
	)

	if req.Labels != nil {
		maps.Copy(labels, req.Labels.Labels)
	}

	return &machineAllocationSpec{
		allocation: metal.MachineAllocation{
			Creator:            creator,
			UUID:               uuid,
			Name:               name,
			Description:        description,
			Hostname:           hostname,
			Project:            req.Project,
			SSHPubKeys:         req.SshPublicKeys,
			UserData:           userdata,
			Role:               role,
			FirewallRules:      fwrules,
			DNSServers:         dnsServers,
			NTPServers:         ntpServers,
			Labels:             labels,
			FilesystemLayoutID: pointer.SafeDeref(req.FilesystemLayout),
		},
		PartitionID:   partitionID,
		Machine:       m,
		Size:          size,
		Image:         image,
		Networks:      req.Networks,
		PlacementTags: req.PlacementTags,
	}, nil

}

/* TODO remove after impl.
   // PrivatePrimaryUnshared is a network which is for machines which is private
   PrivatePrimaryUnshared = "privateprimaryunshared" => CHILD
   // PrivatePrimaryShared is a network which is for machines which is private and shared for other networks
   // This is the case for firewalls and nodes in storage clusters
   PrivatePrimaryShared = "privateprimaryshared" => CHILD_SHARED
   // PrivateSecondaryShared is a network which is for machines which is consumed from a other shared network
   // This is the case for firewall in other clusters which consume the storage
   PrivateSecondaryShared = "privatesecondaryshared" => CHILD_SHARED if machine/firewall project != network project
*/

func (r *machineRepository) allocateMachine(ctx context.Context, spec *machineAllocationSpec) (allocatedMachine *metal.Machine, err error) {
	var fsl *metal.FilesystemLayout
	if spec.allocation.FilesystemLayoutID == "" {
		var fsls metal.FilesystemLayouts
		fsls, err := r.s.ds.FilesystemLayout().List(ctx, nil)
		if err != nil {
			return nil, err
		}
		fsl, err = fsls.From(spec.Size.ID, spec.Image.ID)
		if err != nil {
			return nil, err
		}
	} else {
		fsl, err = r.s.ds.FilesystemLayout().Get(ctx, spec.allocation.FilesystemLayoutID)
		if err != nil {
			return nil, err
		}
	}

	machineCandidate, err := r.findMachineCandidate(ctx, spec)
	if err != nil {
		return nil, err
	}

	// as some fields in the allocation spec are optional, they will now be clearly defined by the machine candidate
	spec.UUID = machineCandidate.ID

	allocationUUID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("unable to create allocation uuid %w", err)
	}

	alloc := &metal.MachineAllocation{
		UUID:            allocationUUID.String(),
		Created:         time.Now(),
		Creator:         spec.allocation.Creator,
		Name:            spec.allocation.Name,
		Description:     spec.allocation.Description,
		Hostname:        spec.allocation.Hostname,
		Project:         spec.allocation.Project,
		ImageID:         spec.Image.ID,
		UserData:        spec.allocation.UserData,
		SSHPubKeys:      spec.allocation.SSHPubKeys,
		MachineNetworks: []*metal.MachineNetwork{},
		Role:            spec.allocation.Role,
		VPN:             spec.allocation.VPN,
		FirewallRules:   spec.allocation.FirewallRules,
		DNSServers:      spec.allocation.DNSServers,
		NTPServers:      spec.allocation.NTPServers,
		Labels:          spec.allocation.Labels,
	}

	err = fsl.Matches(machineCandidate.Hardware)
	if err != nil {
		return nil, fmt.Errorf("unable to check for fsl match:%w", err)
	}
	alloc.FilesystemLayout = fsl

	networks, err := r.convertToMetalAllocationNetwork(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("unable to gather networks:%w", err)
	}
	err = r.makeNetworks(ctx, spec, networks, alloc)
	if err != nil {
		return nil, fmt.Errorf("unable to make networks:%w", err)
	}

	// refetch the machine to catch possible updates after dealing with the network...
	machine, err := r.s.ds.Machine().Get(ctx, machineCandidate.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to find machine:%w", err)
	}
	if machine.Allocation != nil {
		return nil, fmt.Errorf("machine %q already allocated", machine.ID)
	}

	machine.Allocation = alloc
	machine.Tags = r.makeMachineTags(machine)
	machine.PreAllocated = false

	err = r.s.ds.Machine().Update(ctx, machine)
	if err != nil {
		return nil, fmt.Errorf("error when allocating machine %q, %w", machine.ID, err)
	}

	return machine, nil
}

func (r *machineRepository) findMachineCandidate(ctx context.Context, spec *machineAllocationSpec) (*metal.Machine, error) {
	// TODO remove and do in findWaitingMachine
	var (
		err     error
		machine *metal.Machine
	)
	if spec.Machine == nil {
		// requesting allocation of an arbitrary ready machine in partition with given size
		machine, err = r.findWaitingMachine(ctx, spec)
		if err != nil {
			return nil, err
		}
	} else {
		// requesting allocation of a specific, existing machine
		machine = spec.Machine
	}
	return machine, err
}

// FindWaitingMachine returns an available, not allocated, waiting and alive machine of given size within the given partition.
func (r *machineRepository) findWaitingMachine(ctx context.Context, spec *machineAllocationSpec) (*metal.Machine, error) {
	if err := r.s.ds.Lock(ctx, spec.PartitionID, generic.NewLockOptExpirationTimeout(10*time.Second)); err != nil {
		return nil, fmt.Errorf("too many parallel machine allocations taking place, try again later:%w", err)
	}
	defer r.s.ds.Unlock(ctx, spec.PartitionID)

	candidates, err := r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{
		Partition:    &spec.PartitionID,
		Size:         &spec.Size.ID,
		State:        apiv2.MachineState_MACHINE_STATE_AVAILABLE.Enum(), // Machines which are locked or reserved are not considered
		Waiting:      new(true),
		Preallocated: new(false),
		NotAllocated: new(true),
	}))
	if err != nil {
		return nil, err
	}

	ecs, err := r.s.ds.Event().List(ctx, nil)
	if err != nil {
		return nil, err
	}
	ecMap := metal.ProvisioningEventsByID(ecs)

	var available []*metal.Machine
	for _, m := range candidates {
		ec, ok := ecMap[m.ID]
		if !ok {
			r.s.log.Error("cannot find machine provisioning event container", "machine", m, "error", err)
			// fall through, so the rest of the machines is getting evaluated
			continue
		}
		if ec.Liveliness != metal.MachineLivelinessAlive {
			continue
		}
		available = append(available, m)
	}

	if len(available) == 0 {
		return nil, errors.New("no machine available")
	}

	partitionMachines, err := r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{
		Partition: &spec.PartitionID,
		Size:      &spec.Size.ID,
	}))
	if err != nil {
		return nil, err
	}

	reservations, err := r.s.ds.SizeReservation().List(ctx, queries.SizeReservationFilter(&apiv2.SizeReservationQuery{Partition: &spec.PartitionID, Size: &spec.Size.ID}))
	if err != nil {
		return nil, err
	}

	var (
		machinesByProject = make(map[string][]*metal.Machine)
		projectMachines   []*metal.Machine
	)
	for _, m := range partitionMachines {
		if m.Allocation == nil {
			continue
		}
		machinesByProject[m.Allocation.Project] = append(machinesByProject[m.Allocation.Project], m)
		if m.Allocation.Project == spec.allocation.Project && m.Allocation.Role == spec.allocation.Role {
			projectMachines = append(projectMachines, m)
		}
	}

	ok := r.checkSizeReservations(available, spec.allocation.Project, machinesByProject, reservations)
	if !ok {
		return nil, errors.New("no machine available")
	}

	spreadCandidates := r.spreadAcrossRacks(available, projectMachines, spec.PlacementTags)
	if len(spreadCandidates) == 0 {
		return nil, errors.New("no machine available")
	}

	machine := spreadCandidates[randomIndex(len(spreadCandidates))]
	machine.PreAllocated = true

	err = r.s.ds.Machine().Update(ctx, machine)
	if err != nil {
		return nil, err
	}

	return machine, nil
}

func (r *machineRepository) spreadAcrossRacks(allMachines, projectMachines []*metal.Machine, tags []string) []*metal.Machine {
	var (
		allRacks = groupByRack(allMachines)

		projectRacks                = groupByRack(projectMachines)
		leastOccupiedByProjectRacks = electRacks(allRacks, projectRacks)

		taggedMachines           = groupByTags(projectMachines).filter(tags...).getMachines()
		taggedRacks              = groupByRack(taggedMachines)
		leastOccupiedByTagsRacks = electRacks(allRacks, taggedRacks)

		intersection = intersect(leastOccupiedByTagsRacks, leastOccupiedByProjectRacks)
	)

	if c := allRacks.filter(intersection...).getMachines(); len(c) > 0 {
		return c
	}

	return allRacks.filter(leastOccupiedByTagsRacks...).getMachines() // tags have precedence over project
}

type groupedMachines map[string][]*metal.Machine

func groupByRack(machines []*metal.Machine) groupedMachines {
	racks := make(groupedMachines)

	for _, m := range machines {
		racks[m.RackID] = append(racks[m.RackID], m)
	}

	return racks
}

func (g groupedMachines) getMachines() []*metal.Machine {
	machines := make([]*metal.Machine, 0)

	for id := range g {
		machines = append(machines, g[id]...)
	}

	return machines
}

func (g groupedMachines) filter(keys ...string) groupedMachines {
	result := make(groupedMachines)

	for i := range keys {
		ms, ok := g[keys[i]]
		if ok {
			result[keys[i]] = ms
		}
	}

	return result
}

// electRacks returns the least occupied racks from all racks
func electRacks(allRacks, occupiedRacks groupedMachines) []string {
	winners := make([]string, 0)
	min := math.MaxInt

	for id := range allRacks {
		if _, ok := occupiedRacks[id]; ok {
			continue
		}
		occupiedRacks[id] = nil
	}

	for id := range occupiedRacks {
		if _, ok := allRacks[id]; !ok {
			continue
		}

		switch {
		case len(occupiedRacks[id]) < min:
			min = len(occupiedRacks[id])
			winners = []string{id}
		case len(occupiedRacks[id]) == min:
			winners = append(winners, id)
		}
	}

	return winners
}

func groupByTags(machines []*metal.Machine) groupedMachines {
	groups := make(groupedMachines)

	for _, m := range machines {
		for j := range m.Tags {
			ms := groups[m.Tags[j]]
			groups[m.Tags[j]] = append(ms, m)
		}
	}

	return groups
}

func randomIndex(max int) int {
	if max <= 0 {
		return 0
	}
	// golangci-lint has an issue with math/rand/v2
	// here it provides sufficient randomness though because it's not used for cryptographic purposes
	return rand.N(max) //nolint:gosec
}

func intersect[T comparable](a, b []T) []T {
	c := make([]T, 0)

	for i := range a {
		if slices.Contains(b, a[i]) {
			c = append(c, a[i])
		}
	}

	return c
}

// checkSizeReservations returns true when an allocation is possible and
// false when size reservations prevent the allocation for the given project in the given partition
func (r *machineRepository) checkSizeReservations(available []*metal.Machine, projectid string, machinesByProject map[string][]*metal.Machine, reservations []*metal.SizeReservation) bool {
	if len(reservations) == 0 {
		return true
	}

	var (
		amount = 0
	)

	for _, r := range reservations {
		// sum up the amount of reservations
		amount += r.Amount

		alreadyAllocated := len(machinesByProject[r.ProjectID])

		if projectid == r.ProjectID && alreadyAllocated < r.Amount {
			// allow allocation for the project when it has a reservation and there are still allocations left
			return true
		}

		// subtract already used up reservations of the project
		amount = max(amount-alreadyAllocated, 0)
	}

	return amount < len(available)
}

func (r *machineRepository) convertToMetalAllocationNetwork(ctx context.Context, spec *machineAllocationSpec) ([]*allocationNetwork, error) {
	var (
		specNetworks []*allocationNetwork
	)

	for _, networkSpec := range spec.Networks {
		auto := len(networkSpec.Ips) == 0
		network, err := r.s.ds.Network().Get(ctx, networkSpec.Network)
		if err != nil {
			return nil, err
		}

		n := &allocationNetwork{
			network: network,
			auto:    auto,
			ips:     []*metal.IP{},
		}

		for _, allocationIP := range networkSpec.Ips {
			ip, err := r.s.ds.IP().Get(ctx, metal.CreateNamespacedIPAddress(network.Namespace, allocationIP))
			if err != nil {
				return nil, err
			}
			n.auto = false
			n.ips = append(n.ips, ip)
		}

		specNetworks = append(specNetworks, n)
	}

	// Add underlay to firewall
	if spec.allocation.Role == metal.RoleFirewall {
		underlay, err := r.s.ds.Network().Find(ctx, queries.NetworkFilter(&apiv2.NetworkQuery{
			Partition: &spec.PartitionID,
			Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum(),
		}))
		if err != nil {
			return nil, err
		}

		specNetworks = append(specNetworks, &allocationNetwork{
			network: underlay,
			auto:    true, // FIXME why is this always set to true ?
		})
	}

	return specNetworks, nil
}

// makeNetworks creates network entities and ip addresses as specified in the allocation network map.
// created networks are added to the machine allocation directly after their creation. This way, the rollback mechanism
// is enabled to clean up networks that were already created.
func (r *machineRepository) makeNetworks(ctx context.Context, spec *machineAllocationSpec, networks []*allocationNetwork, alloc *metal.MachineAllocation) error {
	// the metal-networker expects to have the same unique ASN on all networks of this machine
	asn, err := r.acquireASN(ctx)
	if err != nil {
		return err
	}
	for _, n := range networks {
		if n == nil || n.network == nil {
			continue
		}
		machineNetwork, err := r.makeMachineNetwork(ctx, spec, n, *asn)
		if err != nil {
			return err
		}
		alloc.MachineNetworks = append(alloc.MachineNetworks, machineNetwork)
	}

	return nil
}

// FIXME this is the complicated part which needs to be reviewed

func (r *machineRepository) makeMachineNetwork(ctx context.Context, spec *machineAllocationSpec, network *allocationNetwork, asn uint32) (*metal.MachineNetwork, error) {
	if network.auto {
		if len(network.network.Prefixes) == 0 {
			return nil, fmt.Errorf("given network %s does not have prefixes configured", network.network.ID)
		}
		for _, af := range network.network.Prefixes.AddressFamilies() {
			ipAddress, ipParentCidr, err := r.s.IP(spec.allocation.Project).AdditionalMethods().allocateRandomIP(ctx, network.network, af)
			if err != nil {
				return nil, fmt.Errorf("unable to allocate an ip in network: %s %w", network.network.ID, err)
			}
			ip := &metal.IP{
				IPAddress:        ipAddress,
				ParentPrefixCidr: ipParentCidr,
				Name:             spec.allocation.Name,
				Description:      "autoassigned",
				NetworkID:        network.network.ID,
				Type:             metal.Ephemeral,
				ProjectID:        spec.allocation.Project,
			}
			// FIXME ugly implementation
			ip.AddMachineId(spec.UUID)

			_, err = r.s.ds.IP().Create(ctx, ip)
			if err != nil {
				return nil, err
			}
			network.ips = append(network.ips, ip)
		}
	}

	// from the makeNetworks call, a lot of ips might be set in this network
	// add a machine tag to all of them
	var ipAddresses []string
	for _, ip := range network.ips {
		ip.AddMachineId(spec.UUID)
		err := r.s.ds.IP().Update(ctx, ip)
		if err != nil {
			return nil, err
		}
		ipAddresses = append(ipAddresses, ip.IPAddress)
	}

	// FIXME we should return the apiv2.MachineNetwork
	machineNetwork := metal.MachineNetwork{
		NetworkID:           network.network.ID,
		Prefixes:            network.network.Prefixes.String(),
		IPs:                 ipAddresses,
		DestinationPrefixes: network.network.DestinationPrefixes.String(),
		// We do not carry over these old parameters,
		// the new networker must figure out all aspekts from the v2 networktype
		//
		// PrivatePrimary:      n.networkType.PrivatePrimary,
		// Private:             n.networkType.Private,
		// Shared:              n.networkType.Shared,
		// Underlay: underlay,
		// Nat: nat,
		Vrf: network.network.Vrf,
		ASN: asn,
		// New network properties
		ProjectID:   network.network.ProjectID,
		NetworkType: network.network.NetworkType,
		NATType:     network.network.NATType,
	}

	return &machineNetwork, nil
}

// FIXME review machine and allocation labels

// makeMachineTags constructs the tags of the machine.
// - system tags (immutable information from the metal-api that are useful for the end user, e.g. machine rack and chassis)
func (r *machineRepository) makeMachineTags(m *metal.Machine) []string {
	var (
		labels = make(map[string]string)
		tags   []string
	)
	for _, n := range m.Allocation.MachineNetworks {
		if n.ASN != 0 {
			labels[tag.MachineNetworkPrimaryASN] = strconv.FormatInt(int64(n.ASN), 10)
			break
		}
	}

	if m.RackID != "" {
		labels[tag.MachineRack] = m.RackID
	}

	if m.IPMI.Fru.ChassisPartSerial != "" {
		labels[tag.MachineChassis] = m.IPMI.Fru.ChassisPartSerial
	}

	// TODO add more hardware related info
	// TODO add building for metro setups

	for k, v := range labels {
		tags = append(tags, fmt.Sprintf("%s=%s", k, v))
	}

	tags = lo.Uniq(tags)

	return tags
}
