package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/hibiken/asynq"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	asyncclient "github.com/metal-stack/metal-apiserver/pkg/async/client"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type networkRepository struct {
	r     *Store
	scope *ProjectScope
}

func (r *networkRepository) Get(ctx context.Context, id string) (*metal.Network, error) {
	nw, err := r.r.ds.Network().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	err = r.MatchScope(nw)
	if err != nil {
		return nil, err
	}

	return nw, nil
}

func (r *networkRepository) MatchScope(nw *metal.Network) error {
	if r.scope == nil {
		return nil
	}
	eventualNw := pointer.SafeDeref(nw)
	if r.scope.projectID == eventualNw.ProjectID {
		return nil
	}
	return errorutil.NotFound("network:%s project:%s for scope:%s not found", eventualNw.ID, eventualNw.ProjectID, r.scope.projectID)
}

func (r *networkRepository) Delete(ctx context.Context, n *Validated[*metal.Network]) (*metal.Network, error) {
	nw, err := r.Get(ctx, n.message.ID)
	if err != nil {
		return nil, err
	}

	info, err := r.r.async.NewNetworkDeleteTask(nw.ID)
	if err != nil {
		return nil, err
	}
	r.r.log.Info("network delete queued", "info", info)

	return nw, nil
}

// NetworkDeleteHandleFn is called async to ensure all dependent entities are deleted
// Async deletion must be scheduled by async.NewNetworkDeleteTask
func (r *Store) NetworkDeleteHandleFn(ctx context.Context, t *asynq.Task) error {

	var payload asyncclient.NetworkDeletePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %w %w", err, asynq.SkipRetry)
	}
	r.log.Info("delete network handler", "uuid", payload.UUID)

	nw, err := r.ds.Network().Get(ctx, payload.UUID)
	if err != nil && !errorutil.IsNotFound(err) {
		return err
	}

	for _, prefix := range nw.Prefixes {
		_, err = r.ipam.DeletePrefix(ctx, connect.NewRequest(&ipamv1.DeletePrefixRequest{Cidr: prefix.String()}))
		if err != nil && !errorutil.IsNotFound(err) {
			r.log.Error("network release", "error", err)
			return err
		}
	}
	if nw.Vrf > 0 {
		err = r.ds.VrfPool().ReleaseUniqueInteger(ctx, nw.Vrf)
		if err != nil {
			return fmt.Errorf("unable to release vrf:%d %w", nw.Vrf, err)
		}
	}

	err = r.ds.Network().Delete(ctx, nw)
	if err != nil && !errorutil.IsNotFound(err) {
		r.log.Error("network delete", "error", err)
		return err
	}

	return nil
}

func (r *networkRepository) AllocateNetwork(ctx context.Context, rq *Validated[*apiv2.NetworkServiceCreateRequest]) (*metal.Network, error) {
	req := rq.message
	var (
		name        = pointer.SafeDeref(req.Name)
		description = pointer.SafeDeref(req.Description)
		partition   = pointer.SafeDeref(req.Partition)
		labels      map[string]string

		nat    bool
		shared bool

		parent *metal.Network
	)

	if req.Labels != nil {
		labels = req.Labels.Labels
	}
	if req.ParentNetworkId != nil && partition != "" {
		return nil, errorutil.InvalidArgument("either partition or parentnetworkid must be specified")
	}

	childPrefixes, parent, err := r.allocateChildPrefixes(ctx, req.ParentNetworkId, req.Partition, req.Length, req.AddressFamily)
	if err != nil {
		return nil, err
	}

	r.r.log.Debug("acquire network", "child prefixes", childPrefixes)

	vrf, err := r.r.ds.VrfPool().AcquireRandomUniqueInteger(ctx)
	if err != nil {
		return nil, errorutil.Internal("could not acquire a vrf: %w", err)
	}

	// Inherit nat from Parent
	if parent.NATType != nil && *parent.NATType == metal.IPv4MasqueradeNATType {
		nat = true
	}

	networkType := metal.PrivateNetworkType

	nw := &metal.Network{
		Base: metal.Base{
			Name:        name,
			Description: description,
		},
		Prefixes:        childPrefixes,
		PartitionID:     partition,
		ProjectID:       req.Project,
		Nat:             nat,
		PrivateSuper:    false,
		Underlay:        false,
		Shared:          shared,
		Vrf:             vrf,
		ParentNetworkID: parent.ID,
		Labels:          labels,
		NATType:         parent.NATType,
		NetworkType:     &networkType,
	}

	nw, err = r.r.ds.Network().Create(ctx, nw)
	if err != nil {
		return nil, err
	}

	return nw, nil
}

func (r *networkRepository) Create(ctx context.Context, rq *Validated[*adminv2.NetworkServiceCreateRequest]) (*metal.Network, error) {
	req := rq.message
	var (
		id          = pointer.SafeDeref(req.Id)
		name        = pointer.SafeDeref(req.Name)
		description = pointer.SafeDeref(req.Description)
		projectId   = pointer.SafeDeref(req.Project)
		partition   = pointer.SafeDeref(req.Partition)
		labels      map[string]string
		vrf         uint

		nat          bool
		privateSuper bool
		shared       bool
		underlay     bool
	)

	if req.Labels != nil {
		labels = req.Labels.Labels
	}

	prefixes, err := metal.NewPrefixesFromCIDRs(req.Prefixes)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	switch req.Type {
	case apiv2.NetworkType_NETWORK_TYPE_PRIVATE, apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SHARED:
		//
		childPrefixes, parent, err := r.allocateChildPrefixes(ctx, req.ParentNetworkId, req.Partition, req.Length, req.AddressFamily)
		if err != nil {
			return nil, err
		}

		vrf, err := r.r.ds.VrfPool().AcquireRandomUniqueInteger(ctx)
		if err != nil {
			return nil, errorutil.Internal("could not acquire a vrf: %w", err)
		}

		// Inherit nat from Parent
		if parent.NATType != nil && *parent.NATType == metal.IPv4MasqueradeNATType {
			nat = true
		}

		networkType, err := metal.ToNetworkTyp(req.Type)
		if err != nil {
			return nil, errorutil.NewInternal(err)
		}
		if req.Type == apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SHARED {
			shared = true
		}

		nw := &metal.Network{
			Base: metal.Base{
				Name:        name,
				Description: description,
			},
			Prefixes:        childPrefixes,
			PartitionID:     partition,
			ProjectID:       projectId,
			Nat:             nat,
			PrivateSuper:    false,
			Underlay:        false,
			Shared:          shared,
			Vrf:             vrf,
			ParentNetworkID: parent.ID,
			Labels:          labels,
			NATType:         parent.NATType,
			NetworkType:     &networkType,
		}

		nw, err = r.r.ds.Network().Create(ctx, nw)
		if err != nil {
			return nil, err
		}

		return nw, nil

	case apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER, apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER_NAMESPACED:
		privateSuper = true
		//
	case apiv2.NetworkType_NETWORK_TYPE_SUPER_VRF_SHARED:
		//
	case apiv2.NetworkType_NETWORK_TYPE_VRF_SHARED:
		//
	case apiv2.NetworkType_NETWORK_TYPE_SHARED:
		shared = true
		//
	case apiv2.NetworkType_NETWORK_TYPE_UNDERLAY:
		underlay = true
		//
	case apiv2.NetworkType_NETWORK_TYPE_UNSPECIFIED:
		fallthrough
	default:
		return nil, errorutil.InvalidArgument("given networktype:%s is invalid", req.Type)
	}

	if req.NatType != nil && req.NatType == apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum() {
		nat = true
	}

	destPrefixes, err := metal.NewPrefixesFromCIDRs(req.DestinationPrefixes)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	defaultChildPrefixLength, err := metal.ToChildPrefixLength(req.DefaultChildPrefixLength, prefixes)
	if err != nil {
		return nil, errorutil.NewInvalidArgument(err)
	}
	minChildPrefixLength, err := metal.ToChildPrefixLength(req.MinChildPrefixLength, prefixes)
	if err != nil {
		return nil, errorutil.NewInvalidArgument(err)
	}

	if req.Vrf != nil {
		vrf, err = r.r.ds.VrfPool().AcquireUniqueInteger(ctx, uint(*req.Vrf))
		if err != nil {
			if !errorutil.IsConflict(err) {
				return nil, errorutil.InvalidArgument("could not acquire vrf: %w", err)
			}
			// FIXME parent must have this type, backwards compatibility. Should look at parent for private_shared_vrf
			if req.Type != apiv2.NetworkType_NETWORK_TYPE_SUPER_VRF_SHARED {
				return nil, errorutil.InvalidArgument("cannot acquire a unique vrf id twice except parent networktype is %s", apiv2.NetworkType_NETWORK_TYPE_SUPER_VRF_SHARED)
			}
		}
	} else {
		// FIXME in case req.vrf is nil, the network will be created with a nil vrf ?
		// This is the case in the actual metal-api implementation
		// Therefor we create a random vrf instead for private networks
		if !privateSuper {
			vrf, err = r.r.ds.VrfPool().AcquireRandomUniqueInteger(ctx)
			if err != nil {
				return nil, errorutil.Internal("could not acquire a vrf: %w", err)
			}
		}
	}

	var (
		natType *metal.NATType
	)

	if req.NatType != nil {
		nat, err := metal.ToNATType(*req.NatType)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		natType = &nat
	}
	networkType, err := metal.ToNetworkTyp(req.Type)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	nw := &metal.Network{
		Base: metal.Base{
			ID:          id,
			Name:        name,
			Description: description,
		},
		Prefixes:                   prefixes,
		ParentNetworkID:            pointer.SafeDeref(req.ParentNetworkId),
		DestinationPrefixes:        destPrefixes,
		DefaultChildPrefixLength:   defaultChildPrefixLength,
		MinChildPrefixLength:       minChildPrefixLength,
		PartitionID:                partition,
		ProjectID:                  projectId,
		Nat:                        nat,
		PrivateSuper:               privateSuper,
		Underlay:                   underlay,
		Vrf:                        vrf,
		Shared:                     shared,
		Labels:                     labels,
		AdditionalAnnouncableCIDRs: req.AdditionalAnnouncableCidrs,
		NetworkType:                &networkType,
		NATType:                    natType,
	}

	resp, err := r.r.ds.Network().Create(ctx, nw)
	if err != nil {
		return nil, err
	}

	for _, prefix := range nw.Prefixes {
		_, err = r.r.ipam.CreatePrefix(ctx, connect.NewRequest(&ipamv1.CreatePrefixRequest{Cidr: prefix.String()}))
		if err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func (r *networkRepository) Update(ctx context.Context, rq *Validated[*adminv2.NetworkServiceUpdateRequest]) (*metal.Network, error) {
	old, err := r.Get(ctx, rq.message.Id)
	if err != nil {
		return nil, err
	}
	newNetwork := *old
	req := rq.message

	if req.Name != nil {
		newNetwork.Name = *req.Name
	}
	if req.Description != nil {
		newNetwork.Description = *req.Description
	}
	if req.Labels != nil && req.Labels.Labels != nil {
		newNetwork.Labels = req.Labels.Labels
	}
	// Fixme network options
	// if new.Options != nil && new.Options.Shared != nil {
	// 	newNetwork.Shared = *new.Shared
	// }

	var (
		prefixesToBeRemoved metal.Prefixes
		prefixesToBeAdded   metal.Prefixes
	)

	prefixesToBeRemoved, prefixesToBeAdded, err = r.calculatePrefixDifferences(ctx, old, &newNetwork, req.Prefixes)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	if req.DestinationPrefixes != nil {
		destPrefixes, err := metal.NewPrefixesFromCIDRs(req.DestinationPrefixes)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		newNetwork.DestinationPrefixes = destPrefixes
	}

	if req.DefaultChildPrefixLength != nil {
		newDefaultChildPrefixLength, err := metal.ToChildPrefixLength(req.DefaultChildPrefixLength, newNetwork.Prefixes)
		if err != nil {
			return nil, errorutil.NewInvalidArgument(err)
		}
		newNetwork.DefaultChildPrefixLength = newDefaultChildPrefixLength
	}

	if req.Force {
		newNetwork.AdditionalAnnouncableCIDRs = req.AdditionalAnnouncableCidrs
	}

	r.r.log.Debug("update", "network id", newNetwork.ID, "prefixes to add", prefixesToBeAdded, "prefixes to remove", prefixesToBeRemoved)

	for _, p := range prefixesToBeRemoved {
		_, err := r.r.ipam.DeletePrefix(ctx, connect.NewRequest(&ipamv1.DeletePrefixRequest{Cidr: p.String()}))
		if err != nil {
			return nil, errorutil.Convert(err)
		}
	}

	for _, p := range prefixesToBeAdded {
		_, err := r.r.ipam.CreatePrefix(ctx, connect.NewRequest(&ipamv1.CreatePrefixRequest{Cidr: p.String()}))
		if err != nil {
			return nil, errorutil.Convert(err)
		}
	}

	r.r.log.Debug("updated network", "old", old, "new", newNetwork)
	newNetwork.SetChanged(old.Changed)
	err = r.r.ds.Network().Update(ctx, &newNetwork)
	if err != nil {
		return nil, err
	}

	return &newNetwork, nil
}

func (r *networkRepository) Find(ctx context.Context, query *apiv2.NetworkQuery) (*metal.Network, error) {
	nw, err := r.r.ds.Network().Find(ctx, r.scopedNetworkFilters(queries.NetworkFilter(query))...)
	if err != nil {
		return nil, err
	}

	return nw, nil
}
func (r *networkRepository) List(ctx context.Context, query *apiv2.NetworkQuery) ([]*metal.Network, error) {
	nws, err := r.r.ds.Network().List(ctx, r.scopedNetworkFilters(queries.NetworkFilter(query))...)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(nws, func(a, b *metal.Network) int {
		return strings.Compare(a.ID, b.ID)
	})

	return nws, nil
}
func (r *networkRepository) ConvertToInternal(msg *apiv2.Network) (*metal.Network, error) {
	panic("unimplemented")
}
func (r *networkRepository) ConvertToProto(e *metal.Network) (*apiv2.Network, error) {
	var (
		consumption *apiv2.NetworkConsumption
		labels      *apiv2.Labels
		networkType *apiv2.NetworkType
		natType     *apiv2.NATType
	)

	if e == nil {
		return nil, nil
	}

	if e.Labels != nil {
		labels = &apiv2.Labels{
			Labels: e.Labels,
		}
	}

	if e.Nat {
		natType = apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum()
	}
	if e.PrivateSuper {
		networkType = apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER.Enum()
	}
	if e.Shared {
		networkType = apiv2.NetworkType_NETWORK_TYPE_SHARED.Enum()
	}
	if e.Underlay {
		networkType = apiv2.NetworkType_NETWORK_TYPE_UNDERLAY.Enum()
	}
	if e.ParentNetworkID != "" {
		networkType = apiv2.NetworkType_NETWORK_TYPE_PRIVATE.Enum()
	}
	if e.NetworkType != nil {
		nwt, err := metal.FromNetworkTyp(*e.NetworkType)
		if err != nil {
			return nil, err
		}
		networkType = &nwt
	}
	// TODO: how to detect private super shared vrf

	defaultChildPrefixLength, err := r.toProtoChildPrefixLength(e.DefaultChildPrefixLength)
	if err != nil {
		return nil, err
	}
	minChildPrefixLength, err := r.toProtoChildPrefixLength(e.MinChildPrefixLength)
	if err != nil {
		return nil, err
	}

	nw := &apiv2.Network{
		Id:                         e.ID,
		Name:                       pointer.PointerOrNil(e.Name),
		Description:                pointer.PointerOrNil(e.Description),
		Partition:                  pointer.PointerOrNil(e.PartitionID),
		Project:                    pointer.PointerOrNil(e.ProjectID),
		Prefixes:                   e.Prefixes.String(),
		DestinationPrefixes:        e.DestinationPrefixes.String(),
		Vrf:                        pointer.PointerOrNil(uint32(e.Vrf)),
		ParentNetworkId:            pointer.PointerOrNil(e.ParentNetworkID),
		AdditionalAnnouncableCidrs: e.AdditionalAnnouncableCIDRs,
		Meta: &apiv2.Meta{
			Labels:    labels,
			CreatedAt: timestamppb.New(e.Created),
			UpdatedAt: timestamppb.New(e.Changed),
		},
		NatType:                  natType,
		DefaultChildPrefixLength: defaultChildPrefixLength,
		MinChildPrefixLength:     minChildPrefixLength,
		Type:                     networkType,
	}
	consumption, err = r.GetNetworkUsage(context.Background(), e)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	nw.Consumption = consumption

	return nw, nil
}

func (r *networkRepository) toProtoChildPrefixLength(childPrefixLength metal.ChildPrefixLength) (*apiv2.ChildPrefixLength, error) {
	var result *apiv2.ChildPrefixLength
	for af, length := range childPrefixLength {
		if result == nil {
			result = &apiv2.ChildPrefixLength{}
		}
		switch af {
		case metal.IPv4AddressFamily:
			result.Ipv4 = pointer.Pointer(uint32(length))
		case metal.IPv6AddressFamily:
			result.Ipv6 = pointer.Pointer(uint32(length))
		default:
			return nil, errorutil.InvalidArgument("unknown addressfamily %s", af)
		}
	}
	return result, nil
}

func (r *networkRepository) GetNetworkUsage(ctx context.Context, nw *metal.Network) (*apiv2.NetworkConsumption, error) {
	consumption := &apiv2.NetworkConsumption{}
	if nw == nil {
		return consumption, nil
	}
	for _, prefix := range nw.Prefixes {
		pfx, err := netip.ParsePrefix(prefix.String())
		if err != nil {
			return nil, err
		}
		af := metal.IPv4AddressFamily
		if pfx.Addr().Is6() {
			af = metal.IPv6AddressFamily
		}
		resp, err := r.r.ipam.PrefixUsage(ctx, connect.NewRequest(&ipamv1.PrefixUsageRequest{Cidr: prefix.String()}))
		if err != nil {
			return nil, err
		}
		u := resp.Msg
		switch af {
		case metal.IPv4AddressFamily:
			if consumption.Ipv4 == nil {
				consumption.Ipv4 = &apiv2.NetworkUsage{}
			}
			consumption.Ipv4.AvailableIps += u.AvailableIps
			consumption.Ipv4.UsedIps += u.AcquiredIps
			consumption.Ipv4.AvailablePrefixes += uint64(len(u.AvailablePrefixes))
			consumption.Ipv4.UsedPrefixes += u.AcquiredPrefixes
		case metal.IPv6AddressFamily:
			if consumption.Ipv6 == nil {
				consumption.Ipv6 = &apiv2.NetworkUsage{}
			}
			consumption.Ipv6.AvailableIps += u.AvailableIps
			consumption.Ipv6.UsedIps += u.AcquiredIps
			consumption.Ipv6.AvailablePrefixes += uint64(len(u.AvailablePrefixes))
			consumption.Ipv6.UsedPrefixes += u.AcquiredPrefixes
		case metal.InvalidAddressFamily:
			return nil, fmt.Errorf("given addressfamily is invalid:%s", af)
		}

	}
	return consumption, nil
}

func (r *networkRepository) scopedNetworkFilters(filter generic.EntityQuery) []generic.EntityQuery {
	var qs []generic.EntityQuery
	if r.scope != nil {
		qs = append(qs, queries.NetworkProjectScoped(r.scope.projectID))
	}
	if filter != nil {
		qs = append(qs, filter)
	}
	return qs
}

func (r *networkRepository) calculatePrefixDifferences(ctx context.Context, existingNetwork, newNetwork *metal.Network, prefixes []string) (toRemoved, toAdded metal.Prefixes, err error) {
	if len(prefixes) == 0 {
		return
	}
	pfxs, err := metal.NewPrefixesFromCIDRs(prefixes)
	if err != nil {
		return nil, nil, err
	}

	toRemoved = existingNetwork.SubtractPrefixes(pfxs...)

	err = r.arePrefixesEmpty(ctx, toRemoved)
	if err != nil {
		return nil, nil, err
	}
	toAdded = newNetwork.SubtractPrefixes(existingNetwork.Prefixes...)
	newNetwork.Prefixes = pfxs
	return toRemoved, toAdded, nil
}

func (r *networkRepository) allocateChildPrefixes(ctx context.Context, parentNetworkId, partitionId *string, requestedLength *apiv2.ChildPrefixLength, af *apiv2.IPAddressFamily) (metal.Prefixes, *metal.Network, error) {

	var (
		prefixes metal.Prefixes
		parent   *metal.Network
	)

	if parentNetworkId != nil {
		p, err := r.r.UnscopedNetwork().Find(ctx, &apiv2.NetworkQuery{Id: parentNetworkId})
		if err != nil {
			return nil, nil, errorutil.InvalidArgument("unable to find a super network with id:%s %w", *parentNetworkId, err)
		}
		if p.NetworkType == nil {
			return nil, nil, errorutil.InvalidArgument("parent network with id:%s does not have a networktype set", *parentNetworkId)
		}
		switch *p.NetworkType {
		case metal.PrivateSuperNetworkType, metal.PrivateSuperNamespacedNetworkType, metal.SuperVrfSharedNetworkType:
			// all good
		default:
			return nil, nil, errorutil.InvalidArgument("parent network with id:%s is not a valid super network but has type:%s", *parentNetworkId, *p.NetworkType)
		}
		parent = p
	} else {
		p, err := r.r.UnscopedNetwork().Find(ctx, &apiv2.NetworkQuery{
			Partition: partitionId,
			Type:      apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER.Enum(),
		})
		if err != nil {
			return nil, nil, errorutil.InvalidArgument("unable to find a private super in partition:%s %w", *partitionId, err)
		}
		parent = p
	}

	// TODO implement namespaced child networks

	length := parent.DefaultChildPrefixLength
	if requestedLength != nil {
		l, err := metal.ToChildPrefixLength(requestedLength, parent.Prefixes)
		if err != nil {
			return nil, nil, errorutil.NewInvalidArgument(err)
		}
		length = l
	}

	if af != nil {
		addressfamily, err := metal.ToAddressFamily(*af)
		if err != nil {
			return nil, nil, errorutil.InvalidArgument("addressfamily is invalid %w", err)
		}
		bits, ok := length[addressfamily]
		if !ok {
			return nil, nil, errorutil.InvalidArgument("addressfamily %s specified, but no childprefixlength for this addressfamily", *af)
		}
		length = metal.ChildPrefixLength{
			addressfamily: bits,
		}
	}

	for af, l := range length {
		childPrefix, err := r.createChildPrefix(ctx, parent.Prefixes, af, l)
		if err != nil {
			return nil, nil, err
		}
		prefixes = append(prefixes, *childPrefix)
	}

	return prefixes, parent, nil
}

func (r *networkRepository) createChildPrefix(ctx context.Context, parentPrefixes metal.Prefixes, af metal.AddressFamily, childLength uint8) (*metal.Prefix, error) {
	var (
		errs []error
	)

	for _, parentPrefix := range parentPrefixes.OfFamily(af) {
		resp, err := r.r.ipam.AcquireChildPrefix(ctx, connect.NewRequest(&ipamv1.AcquireChildPrefixRequest{
			Cidr:   parentPrefix.String(),
			Length: uint32(childLength),
		}))
		if err != nil {
			if errorutil.IsNotFound(err) {
				continue
			}
			errs = append(errs, err)
			continue
		}

		pfx, _, err := metal.NewPrefixFromCIDR(resp.Msg.Prefix.Cidr)
		if err != nil {
			return nil, errorutil.NewInternal(err)
		}
		return pfx, nil
	}

	if len(errs) > 0 {
		return nil, errorutil.Internal("cannot allocate free child prefix in ipam: %w", errors.Join(errs...))
	}

	return nil, errorutil.Internal("cannot allocate free child prefix in one of the given parent prefixes in ipam: %s", parentPrefixes.String())
}
