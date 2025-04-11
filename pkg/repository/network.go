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
	goipam "github.com/metal-stack/go-ipam"
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

func (r *networkRepository) ValidateCreate(ctx context.Context, req *adminv2.NetworkServiceCreateRequest) (*Validated[*adminv2.NetworkServiceCreateRequest], error) {
	r.r.log.Debug("validate create", "req", req)
	if req.Id != nil {
		_, err := r.Get(ctx, *req.Id)
		if err != nil && !errorutil.IsNotFound(err) {
			return nil, errorutil.Conflict("network with id:%s already exists", *req.Id)
		}
	}

	if req.Project != nil {
		_, err := r.r.UnscopedProject().Get(ctx, *req.Project)
		if err != nil {
			return nil, err
		}
	}

	if req.Partition != nil {
		_, err := r.r.Partition().Get(ctx, *req.Partition)
		if err != nil {
			return nil, err
		}
	}

	var (
		nat          bool
		privateSuper bool
		private      bool
	)

	switch req.Type {
	case apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER:
		privateSuper = true
		if err := r.networkTypeInPartitionPossible(ctx, req.Partition, &req.Type); err != nil {
			return nil, err
		}
	case apiv2.NetworkType_NETWORK_TYPE_SUPER_VRF_SHARED:
		privateSuper = true
		// can have multiple super networks
	case apiv2.NetworkType_NETWORK_TYPE_PRIVATE, apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SHARED:
		private = true
	case apiv2.NetworkType_NETWORK_TYPE_SHARED, apiv2.NetworkType_NETWORK_TYPE_UNSPECIFIED:
		// If no networktype was specified, we assume shared
		// shared = true
	case apiv2.NetworkType_NETWORK_TYPE_UNDERLAY:
		// underlay = true
		if err := r.networkTypeInPartitionPossible(ctx, req.Partition, &req.Type); err != nil {
			return nil, err
		}
	default:
		return nil, errorutil.InvalidArgument("given networktype:%s is invalid", req.Type)
	}

	if req.NatType != nil && *req.NatType == apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE {
		nat = true
	}

	if !private && len(req.Prefixes) == 0 {
		return nil, errorutil.InvalidArgument("no prefixes given")
	}

	if !privateSuper && (req.DefaultChildPrefixLength != nil) {
		return nil, errorutil.InvalidArgument("defaultchildprefixlength can only be set for privatesuper networks")
	}

	if nat && (req.Type == apiv2.NetworkType_NETWORK_TYPE_UNDERLAY || req.Type == apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER) {
		return nil, errorutil.InvalidArgument("network with type:%s does not support nat", req.Type)
	}

	prefixes, err := metal.NewPrefixesFromCIDRs(req.Prefixes)
	if err != nil {
		return nil, errorutil.NewInvalidArgument(err)
	}
	destPrefixes, err := metal.NewPrefixesFromCIDRs(req.DestinationPrefixes)
	if err != nil {
		return nil, errorutil.NewInvalidArgument(err)
	}

	defaultChildPrefixLength, err := metal.ToChildPrefixLength(req.DefaultChildPrefixLength, prefixes)
	if err != nil {
		return nil, errorutil.NewInvalidArgument(err)
	}
	_, err = metal.ToChildPrefixLength(req.MinChildPrefixLength, prefixes)
	if err != nil {
		return nil, errorutil.NewInvalidArgument(err)
	}

	err = r.validatePrefixesAndAddressFamilies(prefixes, destPrefixes.AddressFamilies(), defaultChildPrefixLength, privateSuper)
	if err != nil {
		return nil, errorutil.NewInvalidArgument(err)
	}

	err = r.validateAdditionalAnnouncableCIDRs(req.AdditionalAnnouncableCidrs, privateSuper)
	if err != nil {
		return nil, errorutil.NewInvalidArgument(err)
	}

	allNetworks, err := r.List(ctx, &apiv2.NetworkQuery{})
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	existingPrefixes := metal.Prefixes{}
	existingPrefixesMap := make(map[string]bool)
	for _, nw := range allNetworks {
		for _, p := range nw.Prefixes {
			_, ok := existingPrefixesMap[p.String()]
			if !ok {
				existingPrefixes = append(existingPrefixes, p)
				existingPrefixesMap[p.String()] = true
			}
		}
	}

	err = goipam.PrefixesOverlapping(existingPrefixes.String(), prefixes.String())
	if err != nil {
		return nil, errorutil.NewConflict(err)
	}

	return &Validated[*adminv2.NetworkServiceCreateRequest]{
		message: req,
	}, nil
}

func (r *networkRepository) networkTypeInPartitionPossible(ctx context.Context, partition *string, networkType *apiv2.NetworkType) error {
	if partition == nil {
		return nil
	}
	_, err := r.Find(ctx, &apiv2.NetworkQuery{Partition: partition, Type: networkType})
	if err != nil && !errorutil.IsNotFound(err) {
		return errorutil.Convert(err)
	}
	if !errorutil.IsNotFound(err) {
		return errorutil.InvalidArgument("partition with id %q already has a network of type %s", *partition, networkType)
	}
	return nil
}

func (r *networkRepository) validatePrefixesAndAddressFamilies(prefixes metal.Prefixes, destPrefixesAfs metal.AddressFamilies, defaultChildPrefixLength metal.ChildPrefixLength, privateSuper bool) error {
	for _, af := range destPrefixesAfs {
		if !slices.Contains(prefixes.AddressFamilies(), af) {
			return fmt.Errorf("addressfamily:%s of destination prefixes is not present in existing prefixes", af)
		}
	}

	if !privateSuper {
		return nil
	}

	if len(defaultChildPrefixLength) == 0 {
		return fmt.Errorf("private super network must always contain a defaultchildprefixlength")
	}

	for _, af := range prefixes.AddressFamilies() {
		if _, exists := defaultChildPrefixLength[af]; !exists {
			return fmt.Errorf("private super network must always contain a defaultchildprefixlength per addressfamily:%s", af)
		}
	}

	return nil
}

func (r *networkRepository) validateAdditionalAnnouncableCIDRs(additionalCidrs []string, privateSuper bool) error {
	if len(additionalCidrs) == 0 {
		return nil
	}

	if !privateSuper {
		return errors.New("additionalannouncablecidrs can only be set in a private super network")
	}

	for _, cidr := range additionalCidrs {
		_, err := netip.ParsePrefix(cidr)
		if err != nil {
			return fmt.Errorf("given cidr:%q in additionalannouncablecidrs is malformed:%w", cidr, err)
		}
	}

	return nil
}
func (r *networkRepository) ValidateUpdate(ctx context.Context, req *adminv2.NetworkServiceUpdateRequest) (*Validated[*adminv2.NetworkServiceUpdateRequest], error) {
	old, err := r.Get(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	newNetwork := *old

	if req.DefaultChildPrefixLength != nil && !old.PrivateSuper {
		return nil, errorutil.InvalidArgument("defaultchildprefixlength can only be set on privatesuper")
	}

	if old.ParentNetworkID != "" || (old.NetworkType != nil && *old.NetworkType == metal.PrivateNetworkType) {
		if len(req.Prefixes) > 0 {
			return nil, errorutil.InvalidArgument("cannot change prefixes in child networks")
		}
	}

	var (
		prefixesToBeRemoved metal.Prefixes
		prefixesToBeAdded   metal.Prefixes
		destPrefixAfs       metal.AddressFamilies
	)

	prefixesToBeRemoved, prefixesToBeAdded, err = r.calculatePrefixDifferences(ctx, old, &newNetwork, req.Prefixes)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	r.r.log.Debug("validate update", "old parent", old.ParentNetworkID, "prefixes to remove", prefixesToBeRemoved, "prefixes to add", prefixesToBeAdded)
	// Do not allow to change prefixes on child networks
	if old.ParentNetworkID != "" && (len(prefixesToBeRemoved) > 0 || len(prefixesToBeAdded) > 0) {
		return nil, errorutil.InvalidArgument("cannot change prefixes in child networks")
	}

	if req.DestinationPrefixes != nil {
		destPrefixes, err := metal.NewPrefixesFromCIDRs(req.DestinationPrefixes)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		destPrefixAfs = destPrefixes.AddressFamilies()
	}

	err = r.validatePrefixesAndAddressFamilies(newNetwork.Prefixes, destPrefixAfs, old.DefaultChildPrefixLength, old.PrivateSuper)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	_, err = metal.ToChildPrefixLength(req.DefaultChildPrefixLength, newNetwork.Prefixes)
	if err != nil {
		return nil, errorutil.NewInvalidArgument(err)
	}
	_, err = metal.ToChildPrefixLength(req.MinChildPrefixLength, newNetwork.Prefixes)
	if err != nil {
		return nil, errorutil.NewInvalidArgument(err)
	}

	err = r.validateAdditionalAnnouncableCIDRs(req.AdditionalAnnouncableCidrs, old.PrivateSuper)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	for _, oldcidr := range old.AdditionalAnnouncableCIDRs {
		if !req.Force && !slices.Contains(req.AdditionalAnnouncableCidrs, oldcidr) {
			return nil, errorutil.InvalidArgument("you cannot remove %q from additionalannouncablecidrs without force flag set", oldcidr)
		}
	}
	r.r.log.Debug("validated update")

	return &Validated[*adminv2.NetworkServiceUpdateRequest]{
		message: req,
	}, nil
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

func (r *networkRepository) arePrefixesEmpty(ctx context.Context, prefixes metal.Prefixes) error {
	for _, prefixToCheck := range prefixes {
		ips, err := r.r.UnscopedIP().List(ctx, &apiv2.IPQuery{ParentPrefixCidr: pointer.Pointer(prefixToCheck.String())})
		if err != nil {
			return errorutil.Convert(err)
		}
		if len(ips) > 0 {
			return errorutil.InvalidArgument("there are still %d ips present in one of the prefixes:%s", len(ips), prefixToCheck)
		}
	}
	return nil
}

func (r *networkRepository) ValidateDelete(ctx context.Context, req *metal.Network) (*Validated[*metal.Network], error) {
	old, err := r.Get(ctx, req.ID)
	if err != nil {
		if errorutil.IsNotFound(err) {
			return &Validated[*metal.Network]{
				message: req,
			}, nil
		}
		return nil, err
	}

	children, err := r.List(ctx, &apiv2.NetworkQuery{ParentNetworkId: &req.ID})
	if err != nil {
		return nil, err
	}
	if len(children) > 0 {
		return nil, errorutil.InvalidArgument("cannot remove network with existing child networks")
	}

	err = r.arePrefixesEmpty(ctx, old.Prefixes)
	if err != nil {
		return nil, err
	}

	return &Validated[*metal.Network]{
		message: req,
	}, nil
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

func (r *networkRepository) ValidateAllocateNetwork(ctx context.Context, rq *apiv2.NetworkServiceCreateRequest) (*Validated[*apiv2.NetworkServiceCreateRequest], error) {
	if rq.Project == "" {
		return nil, errorutil.InvalidArgument("project must not be empty")
	}
	if rq.Partition == nil {
		return nil, errorutil.InvalidArgument("partition must not be empty")
	}
	_, err := r.r.Project(rq.Project).Get(ctx, rq.Project)
	if err != nil {
		return nil, err
	}
	partition, err := r.r.Partition().Get(ctx, *rq.Partition)
	if err != nil {
		return nil, err
	}

	var superNetwork *metal.Network
	if rq.ParentNetworkId != nil {
		// FIXME network type must be shared
		superNetwork, err = r.r.UnscopedNetwork().Get(ctx, *rq.ParentNetworkId)
		if err != nil {
			return nil, err
		}
	} else {
		superNetwork, err = r.r.UnscopedNetwork().Find(ctx, &apiv2.NetworkQuery{
			Partition: &partition.ID,
			Type:      apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER.Enum(),
		})
		if err != nil {
			return nil, errorutil.InvalidArgument("unable to find a private super in partition:%s %w", partition.ID, err)
		}
	}

	if len(superNetwork.DefaultChildPrefixLength) == 0 {
		return nil, errorutil.InvalidArgument("supernetwork %s has no defaultchildprefixlength specified", superNetwork.ID)
	}

	length := superNetwork.DefaultChildPrefixLength
	if rq.Length != nil {
		l, err := metal.ToChildPrefixLength(rq.Length, superNetwork.Prefixes)
		if err != nil {
			return nil, errorutil.NewInvalidArgument(err)
		}
		length = l
	}

	if rq.AddressFamily != nil {
		addressfamily, err := metal.ToAddressFamily(*rq.AddressFamily)
		if err != nil {
			return nil, errorutil.InvalidArgument("addressfamily is invalid %w", err)
		}
		bits, ok := length[addressfamily]
		if !ok {
			return nil, errorutil.InvalidArgument("addressfamily %s specified, but no childprefixlength for this addressfamily", *rq.AddressFamily)
		}
		length = metal.ChildPrefixLength{
			addressfamily: bits,
		}
	}

	err = r.validatePrefixesAndAddressFamilies(superNetwork.Prefixes, nil, length, false)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &Validated[*apiv2.NetworkServiceCreateRequest]{
		message: rq,
	}, nil
}

func (r *networkRepository) AllocateNetwork(ctx context.Context, rq *Validated[*apiv2.NetworkServiceCreateRequest]) (*metal.Network, error) {
	req := rq.message
	var (
		name        string
		description string
		partition   string
		labels      map[string]string

		childPrefixes = metal.Prefixes{}

		nat    bool
		shared bool

		parent      *metal.Network
		networkType metal.NetworkType
	)

	if req.Partition != nil {
		partition = *req.Partition
	}
	if req.Name != nil {
		name = *req.Name
	}
	if req.Description != nil {
		description = *req.Description
	}
	if req.Labels != nil {
		labels = req.Labels.Labels
	}
	if req.ParentNetworkId != nil && partition != "" {
		return nil, errorutil.InvalidArgument("either partition or parentnetworkid must be specified")
	}

	// TODO: if parentNetworkID is given fetch this
	if req.ParentNetworkId != nil {
		p, err := r.r.UnscopedNetwork().Find(ctx, &apiv2.NetworkQuery{
			Id:   req.ParentNetworkId,
			Type: apiv2.NetworkType_NETWORK_TYPE_SUPER_VRF_SHARED.Enum(),
		})
		if err != nil {
			return nil, errorutil.InvalidArgument("unable to find a super network with id:%s %w", *req.ParentNetworkId, err)
		}
		parent = p
		networkType = metal.VrfSharedNetworkType
	} else {
		p, err := r.r.UnscopedNetwork().Find(ctx, &apiv2.NetworkQuery{
			Partition: &partition,
			Type:      apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER.Enum(),
		})
		if err != nil {
			return nil, errorutil.InvalidArgument("unable to find a private super in partition:%s %w", partition, err)
		}
		parent = p
		networkType = metal.PrivateNetworkType
	}

	if parent.NATType != nil && *parent.NATType == metal.IPv4MasqueradeNATType {
		nat = true
	}

	vrf, err := r.r.ds.VrfPool().AcquireRandomUniqueInteger(ctx)
	if err != nil {
		return nil, errorutil.Internal("could not acquire a vrf: %w", err)
	}

	length := parent.DefaultChildPrefixLength
	if req.Length != nil {
		l, err := metal.ToChildPrefixLength(req.Length, parent.Prefixes)
		if err != nil {
			return nil, errorutil.NewInvalidArgument(err)
		}
		length = l
	}

	if req.AddressFamily != nil {
		addressfamily, err := metal.ToAddressFamily(*req.AddressFamily)
		if err != nil {
			return nil, errorutil.InvalidArgument("addressfamily is invalid %w", err)
		}
		bits, ok := length[addressfamily]
		if !ok {
			return nil, errorutil.InvalidArgument("addressfamily %s specified, but no childprefixlength for this addressfamily", *req.AddressFamily)
		}
		length = metal.ChildPrefixLength{
			addressfamily: bits,
		}
	}

	for af, l := range length {
		childPrefix, err := r.createChildPrefix(ctx, parent.Prefixes, af, l)
		if err != nil {
			return nil, err
		}
		childPrefixes = append(childPrefixes, *childPrefix)
	}
	r.r.log.Debug("acquire network", "child prefixes", childPrefixes)

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

func (r *networkRepository) Create(ctx context.Context, rq *Validated[*adminv2.NetworkServiceCreateRequest]) (*metal.Network, error) {
	req := rq.message
	var (
		id          string
		name        string
		description string
		projectId   string
		partition   string
		labels      map[string]string
		vrf         uint

		nat bool
	)

	if req.Id != nil {
		id = *req.Id
	}
	if req.Project != nil {
		projectId = *req.Project
	}
	if req.Partition != nil {
		partition = *req.Partition
	}
	if req.Name != nil {
		name = *req.Name
	}
	if req.Description != nil {
		description = *req.Description
	}
	if req.Labels != nil {
		labels = req.Labels.Labels
	}

	var (
		privateSuper bool
		shared       bool
		underlay     bool
	)

	switch req.Type {
	case apiv2.NetworkType_NETWORK_TYPE_SHARED, apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SHARED, apiv2.NetworkType_NETWORK_TYPE_UNSPECIFIED:
		// If no network type was specified, we assume shared
		shared = true
	case apiv2.NetworkType_NETWORK_TYPE_PRIVATE_SUPER:
		privateSuper = true
	case apiv2.NetworkType_NETWORK_TYPE_SUPER_VRF_SHARED:
		privateSuper = true
	case apiv2.NetworkType_NETWORK_TYPE_PRIVATE:
		//
	case apiv2.NetworkType_NETWORK_TYPE_UNDERLAY:
		underlay = true
	default:
		return nil, errorutil.InvalidArgument("given networktype:%s is invalid", req.Type)
	}

	if req.NatType != nil && req.NatType == apiv2.NATType_NAT_TYPE_IPV4_MASQUERADE.Enum() {
		nat = true
	}

	prefixes, err := metal.NewPrefixesFromCIDRs(req.Prefixes)
	if err != nil {
		return nil, errorutil.Convert(err)
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
		// FIXME in case req.vrf is nil, the network will be created with a 0 vrf ?
		// This is the case in the actual metal-api implementation
		// Therefor we create a random vrf instead
		vrf, err = r.r.ds.VrfPool().AcquireRandomUniqueInteger(ctx)
		if err != nil {
			return nil, errorutil.Internal("could not acquire a vrf: %w", err)
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
		switch *e.NetworkType {
		case metal.PrivateNetworkType:
			networkType = apiv2.NetworkType_NETWORK_TYPE_PRIVATE.Enum()
		}
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
