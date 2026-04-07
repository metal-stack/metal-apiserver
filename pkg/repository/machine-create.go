package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/tag"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	metalcommon "github.com/metal-stack/metal-lib/pkg/metal"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/samber/lo"
)

// allocationNetwork is intermediate struct to create machine networks from regular networks during machine allocation
type allocationNetwork struct {
	network *metal.Network
	ips     []*metal.IP
}

func (r *machineRepository) allocateMachine(ctx context.Context, req *apiv2.MachineServiceCreateRequest) (allocatedMachine *metal.Machine, err error) {
	var (
		fsl         *metal.FilesystemLayout
		sizeID      = pointer.SafeDeref(req.Size)
		partitionID = pointer.SafeDeref(req.Partition)
		imageID     = req.Image
		creator     string
		machine     *metal.Machine
		role        = metal.RoleMachine
		fwrules     *metal.FirewallRules
		vpn         *metal.MachineVPN
	)
	// figure out creator
	tok, ok := token.TokenFromContext(ctx)
	if ok {
		creator = tok.User
	} else {
		// TODO can we ensure we get a token with the correct user if called from mcm ?
		// Or is it sufficient if the cluster creator is set correct.
		return nil, errorutil.Unauthenticated("unable to get user from context")
	}

	// Allocation of a specific machine is requested, therefore size and partition are not given, fetch them
	if req.Uuid != nil {
		possibleMachine, err := r.s.ds.Machine().Get(ctx, *req.Uuid)
		if err != nil {
			return nil, err
		}

		machine = possibleMachine
		sizeID = machine.SizeID
		partitionID = machine.PartitionID
	}

	// if image is given full-qualified classification filter is not applied
	_, imageVersion, err := metalcommon.GetOsAndSemverFromImage(req.Image)
	if err != nil {
		return nil, err
	}
	if imageVersion.Patch() == 0 {
		image, err := r.s.Image().AdditionalMethods().GetMostRecentImageFor(ctx, &apiv2.ImageServiceLatestRequest{
			Os:             req.Image,
			Classification: apiv2.ImageClassification_IMAGE_CLASSIFICATION_SUPPORTED.Enum(),
		})
		if err != nil {
			return nil, err
		}
		imageID = image.Id
	}

	if req.FilesystemLayout == nil {
		var fsls metal.FilesystemLayouts
		fsls, err := r.s.ds.FilesystemLayout().List(ctx, nil)
		if err != nil {
			return nil, err
		}
		fsl, err = fsls.From(sizeID, imageID)
		if err != nil {
			return nil, err
		}
	} else {
		fsl, err = r.s.ds.FilesystemLayout().Get(ctx, *req.FilesystemLayout)
		if err != nil {
			return nil, err
		}
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

		// FIXME implement VPN authkey generation by calling the vpn-service
		vpn = &metal.MachineVPN{}
	}

	partition, err := r.s.ds.Partition().Get(ctx, partitionID)
	if err != nil {
		return nil, err
	}

	var (
		dnsServers = partition.DNSServers
		ntpServers = partition.NTPServers
	)

	// DNS and NTP Servers from request have precedence
	if len(req.DnsServers) != 0 {
		dnsServers = appendDNSServers(dnsServers, req.DnsServers)
	}
	if len(req.NtpServers) != 0 {
		ntpServers = appendNTPServers(ntpServers, req.NtpServers)
	}

	if req.Uuid == nil {
		machineCandidate, err := r.findWaitingMachine(ctx, partitionID, req.Project, sizeID, req.PlacementTags, role)
		if err != nil {
			return nil, err
		}
		machine = machineCandidate
	}

	var allocatedIPs []*metal.IP
	defer func() {
		// TODO not sure if rollback is even better to be called from outside, but then a rollbackstruck is required
		if err == nil {
			return
		}
		r.rollback(ctx, machine.ID, allocatedIPs)
	}()

	if machine == nil {
		return nil, fmt.Errorf("no machine found")
	}

	allocationUUID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("unable to create allocation uuid %w", err)
	}

	alloc := &metal.MachineAllocation{
		UUID:            allocationUUID.String(),
		Created:         time.Now(),
		Creator:         creator,
		Name:            req.Name,
		Description:     pointer.SafeDeref(req.Description),
		Hostname:        pointer.SafeDerefOrDefault(req.Hostname, "metal"),
		Project:         req.Project,
		ImageID:         imageID,
		UserData:        pointer.SafeDeref(req.Userdata),
		SSHPubKeys:      req.SshPublicKeys,
		MachineNetworks: []*metal.MachineNetwork{},
		Role:            role,
		VPN:             vpn,
		FirewallRules:   fwrules,
		DNSServers:      dnsServers,
		NTPServers:      ntpServers,
	}

	if req.Labels != nil && req.Labels.Labels != nil {
		alloc.Labels = req.Labels.Labels
	}

	err = fsl.Matches(machine.Hardware)
	if err != nil {
		return nil, fmt.Errorf("unable to check for fsl match:%w", err)
	}
	alloc.FilesystemLayout = fsl

	networks, err := r.convertToMetalAllocationNetwork(ctx, req.Networks, partitionID, role)
	if err != nil {
		return nil, fmt.Errorf("unable to gather networks:%w", err)
	}
	machineNetworks, allocatedIPs, err := r.makeNetworks(ctx, machine.ID, req.Project, req.Name, networks)
	if err != nil {
		return nil, fmt.Errorf("unable to make networks:%w", err)
	}
	alloc.MachineNetworks = append(alloc.MachineNetworks, machineNetworks...)

	// refetch the machine to catch possible updates after dealing with the network...
	machine, err = r.s.ds.Machine().Get(ctx, machine.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to find machine:%w", err)
	}
	if machine.Allocation != nil {
		return nil, fmt.Errorf("machine %q already allocated", machine.ID)
	}

	machine.Allocation = alloc
	machine.PreAllocated = false
	r.addMachineTagsAndLabels(machine)

	err = r.s.ds.Machine().Update(ctx, machine)
	if err != nil {
		return nil, fmt.Errorf("error when allocating machine %q, %w", machine.ID, err)
	}

	return machine, nil
}

func (r *machineRepository) rollback(ctx context.Context, machineId string, allocatedIPs []*metal.IP) {
	// TODO: release ASN
	for _, ip := range allocatedIPs {
		if ip.Type == metal.Ephemeral {
			info, err := r.s.task.NewTask(&task.IPDeletePayload{
				AllocationUUID: ip.AllocationUUID,
				IP:             ip.IPAddress,
				Project:        ip.ProjectID,
			})
			if err != nil {
				r.s.log.Error("unable to start task to delete ip", "error", err)
				continue
			}
			r.s.log.Info("ip delete queued", "info", info)
		} else {
			metalIP, err := r.s.ds.IP().Find(ctx, queries.IpFilter(&apiv2.IPQuery{Uuid: &ip.AllocationUUID}))
			if err != nil {
				r.s.log.Error("unable to get ip", "error", err)
				continue
			}
			metalIP.RemoveMachineId(machineId)
			err = r.s.ds.IP().Update(ctx, metalIP)
			if err != nil {
				r.s.log.Error("unable to remove machine tag from ip", "error", err)
				continue
			}
		}
	}
	metalMachine, err := r.s.ds.Machine().Get(ctx, machineId)
	if err != nil {
		r.s.log.Error("unable to get machine", "error", err)
	}

	for _, nw := range metalMachine.Allocation.MachineNetworks {
		if nw.ASN > ASNBase {
			err = r.releaseASN(ctx, nw.ASN)
			r.s.log.Error("unable to release asn", "error", err)
			break
		}
	}

	metalMachine.PreAllocated = false
	metalMachine.Allocation = nil
	err = r.s.ds.Machine().Update(ctx, metalMachine)
	if err != nil {
		r.s.log.Error("unable to remove preallocated flag and allocation from machine", "error", err)
	}
}

// FindWaitingMachine returns an available, not allocated, waiting and alive machine of given size within the given partition.
func (r *machineRepository) findWaitingMachine(ctx context.Context, partition, project, size string, placementTags []string, role metal.Role) (*metal.Machine, error) {
	if err := r.s.ds.Lock(ctx, partition, generic.NewLockOptExpirationTimeout(10*time.Second)); err != nil {
		return nil, fmt.Errorf("too many parallel machine allocations taking place, try again later:%w", err)
	}
	defer r.s.ds.Unlock(ctx, partition)

	candidates, err := r.s.ds.Machine().List(ctx, queries.MachineFilter(&apiv2.MachineQuery{
		Partition:    &partition,
		Size:         &size,
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
		Partition: &partition,
		Size:      &size,
	}))
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
		if m.Allocation.Project == project && m.Allocation.Role == role {
			projectMachines = append(projectMachines, m)
		}
	}

	reservations, err := r.s.ds.SizeReservation().List(ctx, queries.SizeReservationFilter(&apiv2.SizeReservationQuery{Partition: &partition, Size: &size}))
	if err != nil {
		return nil, err
	}
	// TODO the whole size reservation check belongs to validation and should ideally be exposed in size-reservation.go like so:
	// r.s.ds.SizeReservation().Check(QueryBy: project,partition,size)
	ok := r.checkSizeReservations(available, project, machinesByProject, reservations)
	if !ok {
		return nil, errors.New("no machine available")
	}

	desiredMachine, err := r.selectMachine(available, projectMachines, placementTags)
	if err != nil {
		return nil, err
	}

	machine := desiredMachine
	machine.PreAllocated = true

	err = r.s.ds.Machine().Update(ctx, machine)
	if err != nil {
		return nil, err
	}

	return machine, nil
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

func (r *machineRepository) convertToMetalAllocationNetwork(ctx context.Context, networks []*apiv2.MachineAllocationNetwork, partition string, role metal.Role) ([]*allocationNetwork, error) {
	var (
		specNetworks []*allocationNetwork
	)

	for _, networkSpec := range networks {
		network, err := r.s.ds.Network().Get(ctx, networkSpec.Network)
		if err != nil {
			return nil, err
		}

		n := &allocationNetwork{
			network: network,
			ips:     []*metal.IP{},
		}

		for _, allocationIP := range networkSpec.Ips {
			ip, err := r.s.ds.IP().Get(ctx, metal.CreateNamespacedIPAddress(network.Namespace, allocationIP))
			if err != nil {
				return nil, err
			}
			n.ips = append(n.ips, ip)
		}

		specNetworks = append(specNetworks, n)
	}

	// Add underlay to firewall
	if role == metal.RoleFirewall {
		underlay, err := r.s.ds.Network().Find(ctx, queries.NetworkFilter(&apiv2.NetworkQuery{
			Partition: &partition,
			Type:      apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum(),
		}))
		if err != nil {
			return nil, err
		}

		specNetworks = append(specNetworks, &allocationNetwork{
			network: underlay,
		})
	}

	return specNetworks, nil
}

// makeNetworks creates network entities and ip addresses as specified in the allocation network map.
// created networks are added to the machine allocation directly after their creation. This way, the rollback mechanism
// is enabled to clean up networks that were already created.
func (r *machineRepository) makeNetworks(ctx context.Context, machineUUID, project, name string, networks []*allocationNetwork) ([]*metal.MachineNetwork, []*metal.IP, error) {
	// the metal-networker expects to have the same unique ASN on all networks of this machine
	asn, err := r.acquireASN(ctx)
	if err != nil {
		return nil, nil, err
	}
	var (
		machineNetworks []*metal.MachineNetwork
		allocatedIPs    []*metal.IP
	)
	for _, n := range networks {
		if n == nil || n.network == nil {
			continue
		}
		machineNetwork, allocIPs, err := r.makeMachineNetwork(ctx, machineUUID, project, name, n, *asn)
		if err != nil {
			return nil, nil, err
		}
		machineNetworks = append(machineNetworks, machineNetwork)
		allocatedIPs = append(allocatedIPs, allocIPs...)
	}

	return machineNetworks, allocatedIPs, nil
}

// FIXME this is the complicated part which needs to be reviewed

func (r *machineRepository) makeMachineNetwork(ctx context.Context, machineUUID, project, name string, network *allocationNetwork, asn uint32) (*metal.MachineNetwork, []*metal.IP, error) {
	var allocatedIPs []*metal.IP

	if len(network.ips) == 0 {
		if len(network.network.Prefixes) == 0 {
			return nil, nil, fmt.Errorf("given network %s does not have prefixes configured", network.network.ID)
		}
		for _, af := range network.network.Prefixes.AddressFamilies() {
			ipAddress, ipParentCidr, err := r.s.IP(project).AdditionalMethods().allocateRandomIP(ctx, network.network, af)
			if err != nil {
				return nil, nil, fmt.Errorf("unable to allocate an ip in network: %s %w", network.network.ID, err)
			}
			ip := &metal.IP{
				IPAddress:        ipAddress,
				ParentPrefixCidr: ipParentCidr,
				Name:             name,
				Description:      "autoassigned",
				NetworkID:        network.network.ID,
				Type:             metal.Ephemeral,
				ProjectID:        project,
			}
			_, err = r.s.ds.IP().Create(ctx, ip)
			if err != nil {
				return nil, nil, err
			}
			network.ips = append(network.ips, ip)
		}
	}

	// from the makeNetworks call, a lot of ips might be set in this network
	// add a machine tag to all of them
	var ipAddresses []string
	for _, ip := range network.ips {
		ip.AddMachineId(machineUUID)
		err := r.s.ds.IP().Update(ctx, ip)
		if err != nil {
			return nil, nil, err
		}
		ipAddresses = append(ipAddresses, ip.IPAddress)
		// collect all ips, delete only the ephemeral, remove machine tag from static
		allocatedIPs = append(allocatedIPs, ip)
	}

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

	return &machineNetwork, allocatedIPs, nil
}

// FIXME review machine and allocation labels

// makeMachineTags constructs the tags of the machine.
// - system tags (immutable information from the metal-api that are useful for the end user, e.g. machine rack and chassis)
func (r *machineRepository) addMachineTagsAndLabels(m *metal.Machine) {
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
	m.Tags = tags
}

func appendDNSServers(current metal.DNSServers, requestDNSServers []*apiv2.DNSServer) metal.DNSServers {
	if len(requestDNSServers) == 0 {
		return current
	}
	result := make(metal.DNSServers, 0, len(requestDNSServers))
	for _, s := range requestDNSServers {
		result = append(result, metal.DNSServer{IP: s.Ip})
	}
	return result
}

func appendNTPServers(current []metal.NTPServer, requestNTPServers []*apiv2.NTPServer) []metal.NTPServer {
	if len(requestNTPServers) == 0 {
		return current
	}
	result := make([]metal.NTPServer, 0, len(requestNTPServers))
	for _, s := range requestNTPServers {
		result = append(result, metal.NTPServer{Address: s.Address})
	}
	return result
}
