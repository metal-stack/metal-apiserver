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
	s     *Store
	scope *ProjectScope
}

func (r *networkRepository) get(ctx context.Context, id string) (*metal.Network, error) {
	nw, err := r.s.ds.Network().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return nw, nil
}

func (r *networkRepository) matchScope(nw *metal.Network) bool {
	if r.scope == nil {
		return true
	}

	return r.scope.projectID == pointer.SafeDeref(nw).ProjectID
}

func (r *networkRepository) delete(ctx context.Context, nw *metal.Network) error {
	info, err := r.s.async.NewNetworkDeleteTask(nw.ID)
	if err != nil {
		return err
	}

	r.s.log.Info("network delete queued", "info", info)

	return nil
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
	if errorutil.IsNotFound(err) {
		return nil
	}

	for _, prefix := range nw.Prefixes {
		_, err = r.ipam.DeletePrefix(ctx, connect.NewRequest(&ipamv1.DeletePrefixRequest{Cidr: prefix.String(), Namespace: nw.Namespace}))
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

func (r *networkRepository) create(ctx context.Context, req *adminv2.NetworkServiceCreateRequest) (*metal.Network, error) {
	var (
		id          = pointer.SafeDeref(req.Id)
		name        = pointer.SafeDeref(req.Name)
		description = pointer.SafeDeref(req.Description)
		projectId   = pointer.SafeDeref(req.Project)
		partition   = pointer.SafeDeref(req.Partition)
		labels      map[string]string
		vrf         uint

		nat          bool
		natType      *metal.NATType
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
	networkType, err := metal.ToNetworkType(req.Type)
	if err != nil {
		return nil, errorutil.NewInternal(err)
	}

	switch req.Type {
	case apiv2.NetworkType_NETWORK_TYPE_CHILD, apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED:
		childPrefixes, parent, err := r.allocateChildPrefixes(ctx, req.Project, req.ParentNetworkId, req.Partition, req.Length, req.AddressFamily)
		if err != nil {
			return nil, err
		}

		var vrf uint
		if parent.Vrf > 0 {
			vrf = parent.Vrf
		}

		if vrf == 0 {
			vrf, err = r.s.ds.VrfPool().AcquireRandomUniqueInteger(ctx)
			if err != nil {
				return nil, errorutil.Internal("could not acquire a vrf: %w", err)
			}
		}

		if partition == "" {
			partition = parent.PartitionID
		}

		// Inherit nat from Parent
		if parent.NATType != nil && *parent.NATType == metal.NATTypeIPv4Masquerade {
			nat = true
		}

		if req.Type == apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED {
			shared = true
		}
		var namespace *string
		if *parent.NetworkType == metal.NetworkTypeSuperNamespaced {
			namespace = req.Project
		}

		nw := &metal.Network{
			Base: metal.Base{
				Name:        name,
				Description: description,
			},
			Prefixes:            childPrefixes,
			DestinationPrefixes: parent.DestinationPrefixes,
			PartitionID:         partition,
			ProjectID:           projectId,
			Namespace:           namespace,
			Nat:                 nat,
			PrivateSuper:        false,
			Underlay:            false,
			Shared:              shared,
			Vrf:                 vrf,
			ParentNetworkID:     parent.ID,
			Labels:              labels,
			NATType:             parent.NATType,
			NetworkType:         &networkType,
		}

		nw, err = r.s.ds.Network().Create(ctx, nw)
		if err != nil {
			return nil, err
		}

		return nw, nil

	case apiv2.NetworkType_NETWORK_TYPE_SUPER, apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED:
		privateSuper = true
		//
	case apiv2.NetworkType_NETWORK_TYPE_EXTERNAL:
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

	var (
		defaultChildPrefixLength = metal.ToChildPrefixLength(req.DefaultChildPrefixLength)
		minChildPrefixLength     = metal.ToChildPrefixLength(req.MinChildPrefixLength)
	)

	// Only create a random VRF Id for child networks, all other networks must either specify one, or do not set it at all (underlay, super network)
	if req.Vrf != nil {
		vrf, err = r.s.ds.VrfPool().AcquireUniqueInteger(ctx, uint(*req.Vrf))
		if err != nil {
			return nil, err

		}
	}

	if req.NatType != nil {
		nat, err := metal.ToNATType(*req.NatType)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		natType = &nat
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

	resp, err := r.s.ds.Network().Create(ctx, nw)
	if err != nil {
		return nil, err
	}

	for _, prefix := range nw.Prefixes {
		_, err = r.s.ipam.CreatePrefix(ctx, connect.NewRequest(&ipamv1.CreatePrefixRequest{Cidr: prefix.String(), Namespace: nw.Namespace}))
		if err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func (r *networkRepository) update(ctx context.Context, nw *metal.Network, req *adminv2.NetworkServiceUpdateRequest) (*metal.Network, error) {
	if req.Name != nil {
		nw.Name = *req.Name
	}
	if req.Description != nil {
		nw.Description = *req.Description
	}
	if req.Labels != nil {
		nw.Labels = updateLabelsOnMap(req.Labels, nw.Labels)
	}

	if req.NatType != nil {
		nt, err := metal.ToNATType(*req.NatType)
		if err != nil {
			return nil, err
		}

		nw.NATType = &nt
		switch nt {
		case metal.NATTypeIPv4Masquerade:
			nw.Nat = true // nolint:staticcheck
		case metal.NATTypeNone:
			//
		}
	}

	var (
		err                 error
		prefixesToBeRemoved metal.Prefixes
		prefixesToBeAdded   metal.Prefixes
	)

	// Ensure child networks can be updated without loosing the prefixes.
	if metal.IsChildNetwork(old.NetworkType) {
		req.Prefixes = old.Prefixes.String()
	}

	prefixesToBeRemoved, prefixesToBeAdded, err = r.calculatePrefixDifferences(old, req.Prefixes)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	err = r.arePrefixesEmpty(ctx, prefixesToBeRemoved)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	pfxs, err := metal.NewPrefixesFromCIDRs(req.Prefixes)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	newNetwork.Prefixes = pfxs

	if req.DestinationPrefixes != nil {
		destPrefixes, err := metal.NewPrefixesFromCIDRs(req.DestinationPrefixes)
		if err != nil {
			return nil, errorutil.Convert(err)
		}

		nw.DestinationPrefixes = destPrefixes
	}

	if req.DefaultChildPrefixLength != nil {
		nw.DefaultChildPrefixLength = metal.ToChildPrefixLength(req.DefaultChildPrefixLength)
	}

	if req.Force {
		nw.AdditionalAnnouncableCIDRs = req.AdditionalAnnouncableCidrs
	}

	r.s.log.Debug("update", "network id", nw.ID, "prefixes to add", prefixesToBeAdded, "prefixes to remove", prefixesToBeRemoved)

	for _, p := range prefixesToBeRemoved {
		_, err := r.s.ipam.DeletePrefix(ctx, connect.NewRequest(&ipamv1.DeletePrefixRequest{Cidr: p.String(), Namespace: nw.Namespace}))
		if err != nil {
			return nil, errorutil.Convert(err)
		}
	}

	for _, p := range prefixesToBeAdded {
		_, err := r.s.ipam.CreatePrefix(ctx, connect.NewRequest(&ipamv1.CreatePrefixRequest{Cidr: p.String(), Namespace: nw.Namespace}))
		if err != nil {
			return nil, errorutil.Convert(err)
		}
	}

	if req.Prefixes != nil {
		pfxs, err := metal.NewPrefixesFromCIDRs(req.Prefixes)
		if err != nil {
			return nil, err
		}

		nw.Prefixes = pfxs
	}

	err = r.s.ds.Network().Update(ctx, nw)
	if err != nil {
		return nil, err
	}

	return nw, nil
}

func (r *networkRepository) find(ctx context.Context, query *apiv2.NetworkQuery) (*metal.Network, error) {
	nw, err := r.s.ds.Network().Find(ctx, r.scopedNetworkFilters(queries.NetworkFilter(query))...)
	if err != nil {
		return nil, err
	}

	return nw, nil
}

func (r *networkRepository) list(ctx context.Context, query *apiv2.NetworkQuery) ([]*metal.Network, error) {
	nws, err := r.s.ds.Network().List(ctx, r.scopedNetworkFilters(queries.NetworkFilter(query))...)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(nws, func(a, b *metal.Network) int {
		return strings.Compare(a.ID, b.ID)
	})

	return nws, nil
}
func (r *networkRepository) convertToInternal(msg *apiv2.Network) (*metal.Network, error) {
	panic("unimplemented")
}

func (r *networkRepository) convertToProto(e *metal.Network) (*apiv2.Network, error) {
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

	if e.NATType != nil {
		nt, err := metal.FromNATType(*e.NATType)
		if err != nil {
			return nil, err
		}
		natType = &nt
	}

	if e.NetworkType != nil {
		nwt, err := metal.FromNetworkType(*e.NetworkType)
		if err != nil {
			return nil, err
		}
		networkType = &nwt
	}

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
		Namespace:                  e.Namespace,
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
	consumption, err = r.getNetworkUsage(context.Background(), e)
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
		case metal.AddressFamilyIPv4:
			result.Ipv4 = pointer.Pointer(uint32(length))
		case metal.AddressFamilyIPv6:
			result.Ipv6 = pointer.Pointer(uint32(length))
		default:
			return nil, errorutil.InvalidArgument("unknown addressfamily %s", af)
		}
	}
	return result, nil
}

func (r *networkRepository) getNetworkUsage(ctx context.Context, nw *metal.Network) (*apiv2.NetworkConsumption, error) {
	consumption := &apiv2.NetworkConsumption{}
	if nw == nil {
		return consumption, nil
	}
	for _, prefix := range nw.Prefixes {
		pfx, err := netip.ParsePrefix(prefix.String())
		if err != nil {
			return nil, err
		}
		af := metal.AddressFamilyIPv4
		if pfx.Addr().Is6() {
			af = metal.AddressFamilyIPv6
		}
		resp, err := r.s.ipam.PrefixUsage(ctx, connect.NewRequest(&ipamv1.PrefixUsageRequest{Cidr: prefix.String(), Namespace: nw.Namespace}))
		if err != nil {
			return nil, err
		}
		u := resp.Msg
		switch af {
		case metal.AddressFamilyIPv4:
			if consumption.Ipv4 == nil {
				consumption.Ipv4 = &apiv2.NetworkUsage{}
			}
			consumption.Ipv4.AvailableIps += u.AvailableIps
			consumption.Ipv4.UsedIps += u.AcquiredIps
			consumption.Ipv4.AvailablePrefixes += uint64(len(u.AvailablePrefixes))
			consumption.Ipv4.UsedPrefixes += u.AcquiredPrefixes
		case metal.AddressFamilyIPv6:
			if consumption.Ipv6 == nil {
				consumption.Ipv6 = &apiv2.NetworkUsage{}
			}
			consumption.Ipv6.AvailableIps += u.AvailableIps
			consumption.Ipv6.UsedIps += u.AcquiredIps
			consumption.Ipv6.AvailablePrefixes += uint64(len(u.AvailablePrefixes))
			consumption.Ipv6.UsedPrefixes += u.AcquiredPrefixes
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

func (r *networkRepository) calculatePrefixDifferences(existingNetwork *metal.Network, prefixes []string) (toRemoved, toAdded metal.Prefixes, err error) {
	if len(prefixes) == 0 {
		return
	}

	pfxs, err := metal.NewPrefixesFromCIDRs(newPrefixes)
	if err != nil {
		return nil, nil, err
	}

	toRemoved = existingNetwork.Prefixes.SubtractPrefixes(pfxs...)

	toAdded = pfxs.SubtractPrefixes(existingNetwork.Prefixes...)
	return toRemoved, toAdded, nil
}

func (r *networkRepository) allocateChildPrefixes(ctx context.Context, projectId, parentNetworkId, partitionId *string, requestedLength *apiv2.ChildPrefixLength, af *apiv2.NetworkAddressFamily) (metal.Prefixes, *metal.Network, error) {
	var (
		prefixes  metal.Prefixes
		parent    *metal.Network
		namespace *string
	)

	if parentNetworkId != nil {
		r.s.log.Info("get network", "parent", *parentNetworkId)
		p, err := r.s.UnscopedNetwork().Get(ctx, *parentNetworkId)
		if err != nil {
			return nil, nil, errorutil.InvalidArgument("unable to find a super network with id:%s %w", *parentNetworkId, err)
		}
		if p.NetworkType == nil {
			return nil, nil, errorutil.InvalidArgument("parent network with id:%s does not have a networktype set", *parentNetworkId)
		}
		switch *p.NetworkType {
		case metal.NetworkTypeSuper:
			// all good
		case metal.NetworkTypeSuperNamespaced:
			namespace = projectId
		default:
			return nil, nil, errorutil.InvalidArgument("parent network with id:%s is not a valid super network but has type:%s", *parentNetworkId, *p.NetworkType)
		}
		parent = p
	} else {
		p, err := r.s.UnscopedNetwork().Find(ctx, &apiv2.NetworkQuery{
			Partition: partitionId,
			Type:      apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
		})
		if err != nil {
			return nil, nil, errorutil.InvalidArgument("unable to find a private super in partition:%s %w", *partitionId, err)
		}
		parent = p
	}

	parentLength := parent.DefaultChildPrefixLength
	if requestedLength != nil && (requestedLength.Ipv4 != nil || requestedLength.Ipv6 != nil) {
		// FIXME in case requestedLength is Zero, length is returned zero as well
		parentLength = metal.ToChildPrefixLength(requestedLength)
	}

	if af == nil {
		af = apiv2.NetworkAddressFamily_NETWORK_ADDRESS_FAMILY_DUAL_STACK.Enum()
	}

	metalAddressFamily, err := metal.ToAddressFamilyFromNetwork(*af)
	if err != nil {
		return nil, nil, errorutil.InvalidArgument("%w", err)
	}

	if metalAddressFamily != nil {
		bits, ok := parentLength[*metalAddressFamily]
		if !ok {
			return nil, nil, errorutil.InvalidArgument("addressfamily %s specified, but no childprefixlength for this addressfamily", *af)
		}
		parentLength = metal.ChildPrefixLength{
			*metalAddressFamily: bits,
		}
	}

	for af, l := range parentLength {
		childPrefix, err := r.createChildPrefix(ctx, namespace, parent.Prefixes, af, l)
		if err != nil {
			return nil, nil, errorutil.InvalidArgument("error creating child network in parent %s:%w", parent.ID, err)
		}
		prefixes = append(prefixes, *childPrefix)
	}

	return prefixes, parent, nil
}

func (r *networkRepository) createChildPrefix(ctx context.Context, namespace *string, parentPrefixes metal.Prefixes, af metal.AddressFamily, childLength uint8) (*metal.Prefix, error) {
	var (
		errs []error
	)

	if namespace != nil {
		_, err := r.s.ipam.CreateNamespace(ctx, connect.NewRequest(&ipamv1.CreateNamespaceRequest{Namespace: *namespace}))
		if err != nil {
			return nil, errorutil.Internal("unable to create namespace:%v", err)
		}
		for _, parentPrefix := range parentPrefixes.OfFamily(af) {
			_, err := r.s.ipam.GetPrefix(ctx, connect.NewRequest(&ipamv1.GetPrefixRequest{
				Cidr:      parentPrefix.String(),
				Namespace: namespace,
			}))
			if err == nil {
				continue
			}
			if !errorutil.IsNotFound(err) {
				return nil, errorutil.Internal("unable to get prefix %s from super network in ipam:%v", parentPrefix.String(), err)
			}

			_, err = r.s.ipam.CreatePrefix(ctx, connect.NewRequest(&ipamv1.CreatePrefixRequest{
				Cidr:      parentPrefix.String(),
				Namespace: namespace,
			}))
			if err != nil {
				return nil, errorutil.Internal("unable to create namespaced super network:%v", err)
			}
		}
	}
	for _, parentPrefix := range parentPrefixes.OfFamily(af) {
		resp, err := r.s.ipam.AcquireChildPrefix(ctx, connect.NewRequest(&ipamv1.AcquireChildPrefixRequest{
			Cidr:      parentPrefix.String(),
			Length:    uint32(childLength),
			Namespace: namespace,
		}))
		if err != nil {
			// If in one of the prefixes is not enough room for a prefix, ignore it an proceed
			// hopefully one of the prefixes has one left.
			if errorutil.IsNotFound(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("unable to acquire child prefix:%v", err))
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
