package repository

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	goipam "github.com/metal-stack/go-ipam"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

func (r *networkRepository) ValidateCreate(ctx context.Context, req *adminv2.NetworkServiceCreateRequest) (*Validated[*adminv2.NetworkServiceCreateRequest], error) {
	r.r.log.Debug("validate create", "req", req)
	if req.Id != nil {
		if err := r.r.ds.Network().CheckAlreadyExists(ctx, *req.Id); err != nil {
			return nil, err
		}
	}

	if req.Project != nil {
		if _, err := r.r.UnscopedProject().Get(ctx, *req.Project); err != nil {
			return nil, err
		}
	}

	if req.Partition != nil {
		if _, err := r.r.Partition().Get(ctx, *req.Partition); err != nil {
			return nil, err
		}
	}

	var (
		err error
	)

	switch req.Type {
	case apiv2.NetworkType_NETWORK_TYPE_SUPER, apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED:
		err = r.validateCreateNetworkTypeSuper(ctx, req)
	case apiv2.NetworkType_NETWORK_TYPE_CHILD, apiv2.NetworkType_NETWORK_TYPE_CHILD_SHARED:
		err = r.validateCreateNetworkTypeChild(ctx, req)
	case apiv2.NetworkType_NETWORK_TYPE_EXTERNAL:
		err = r.validateCreateNetworkTypeExternal(ctx, req)
	case apiv2.NetworkType_NETWORK_TYPE_UNDERLAY:
		err = r.validateCreateNetworkTypeUnderlay(ctx, req)
	case apiv2.NetworkType_NETWORK_TYPE_UNSPECIFIED:
		fallthrough
	default:
		return nil, errorutil.InvalidArgument("given networktype:%s is invalid", req.Type)
	}

	if err != nil {
		return nil, err
	}

	return &Validated[*adminv2.NetworkServiceCreateRequest]{
		message: req,
	}, nil
}

func (r *networkRepository) validateCreateNetworkTypeChild(ctx context.Context, req *adminv2.NetworkServiceCreateRequest) error {
	// id must be nil
	// if partition is not nil, a super in this partition must be present and is used
	// if partition is nil, a superNamespaces must be present and is used
	// project is mandatory
	// parent network id is optional, if not given, exactly one private super must be found before
	// nat is optional
	// shared is optional
	// if length is given, must not be smaller than min child prefix length
	// requested addressfamily must be possible
	// prefixes must not be specified
	// destination prefixes must not be specified but inherited from super
	// additional announcable cidrs must not be specified
	// vrf can be specified, if the super has vrf specified it will be inherited.
	// if vrf in the super is nil and vrf is nil it will be created from the vrf pool, otherwise the given vrf will be used (formerly known as shared-vrf)
	// defaultchildprefixlength and minchildprefixlength must not be specified

	var errs []error

	errs = validate(errs, req.Project != nil, "project must not be nil")

	errs = validate(errs, req.Id == nil, "id must be nil")
	errs = validate(errs, req.Prefixes == nil, "prefixes must be nil")
	errs = validate(errs, req.DestinationPrefixes == nil, "destination prefixes must be nil")
	errs = validate(errs, req.AdditionalAnnouncableCidrs == nil, "additional announcable cidrs must be nil")
	errs = validate(errs, req.DefaultChildPrefixLength == nil, "default child prefix length must be nil")
	errs = validate(errs, req.MinChildPrefixLength == nil, "min child prefix length length must be nil")
	errs = validate(errs, req.MinChildPrefixLength == nil, "min child prefix length length must be nil")

	if len(errs) > 0 {
		return errorutil.NewInvalidArgument(errors.Join(errs...))
	}

	if req.Partition != nil && req.ParentNetworkId != nil {
		return errorutil.InvalidArgument("if parent network id is specified, partition must be nil")
	}

	var (
		parentNetwork *metal.Network
	)
	if req.ParentNetworkId != nil {
		parent, err := r.r.UnscopedNetwork().Get(ctx, *req.ParentNetworkId)
		if err != nil {
			return errorutil.Convert(fmt.Errorf("unable to retrieve parent network: %w", err))
		}

		switch *parent.NetworkType {
		case metal.SuperNetworkType, metal.SuperNamespacedNetworkType:
			// all good
		case metal.ChildNetworkType, metal.ChildSharedNetworkType, metal.ExternalNetworkType, metal.UnderlayNetworkType:
			return errorutil.InvalidArgument("given parentnetwork must be either a super or a super namespace network")
		}

		if parent.ProjectID != "" && parent.ProjectID != *req.Project {
			return errorutil.InvalidArgument("child network creation not allowed in network %s", *req.ParentNetworkId)
		}

		parentNetwork = parent
	}

	if req.Partition != nil {
		parent, err := r.r.UnscopedNetwork().Find(ctx, &apiv2.NetworkQuery{
			Partition: req.Partition,
			Project:   pointer.Pointer(""),
			Type:      apiv2.NetworkType_NETWORK_TYPE_SUPER.Enum(),
		})
		if err != nil {
			return errorutil.InvalidArgument("unable to find a super in partition:%s %w", *req.Partition, err)
		}
		parentNetwork = parent
	}
	if parentNetwork == nil {
		return errorutil.InvalidArgument("no parent network found")
	}
	if len(parentNetwork.DefaultChildPrefixLength) == 0 {
		return errorutil.InvalidArgument("super network %s has no default child prefix length specified", parentNetwork.ID)
	}
	if parentNetwork.Vrf != 0 && req.Vrf != nil {
		return errorutil.InvalidArgument("super network %q inherits vrf %d to its child networks, therefore the vrf must be nil", parentNetwork.ID, parentNetwork.Vrf)
	}
	if parentNetwork.ProjectID != "" && (parentNetwork.ProjectID != *req.Project) {
		return errorutil.InvalidArgument("super network %s is project scoped, requested child project:%s does not match", parentNetwork.ID, *req.Project)
	}

	length := parentNetwork.DefaultChildPrefixLength
	if req.Length != nil {
		l, err := metal.ToChildPrefixLength(req.Length, parentNetwork.Prefixes)
		if err != nil {
			return errorutil.NewInvalidArgument(err)
		}

		for af, ml := range parentNetwork.MinChildPrefixLength {
			cl, ok := l[af]
			if !ok {
				continue
			}
			if cl < ml {
				return errorutil.InvalidArgument("requested prefix length %v is smaller than allowed (super network defines a minimum of %v)", l, parentNetwork.MinChildPrefixLength)
			}
		}

		length = l
	}

	if req.AddressFamily != nil {
		addressfamily, err := metal.ToAddressFamily(*req.AddressFamily)
		if err != nil {
			return errorutil.InvalidArgument("addressfamily is invalid %w", err)
		}
		bits, ok := length[addressfamily]
		if !ok {
			return errorutil.InvalidArgument("addressfamily %s specified, but no childprefixlength for this addressfamily", *req.AddressFamily)
		}
		length = metal.ChildPrefixLength{
			addressfamily: bits,
		}
	}

	if err := r.validatePrefixesAndAddressFamilies(parentNetwork.Prefixes, nil, length, false); err != nil {
		return errorutil.Convert(err)
	}
	return nil
}

func (r *networkRepository) validateCreateNetworkTypeSuper(ctx context.Context, req *adminv2.NetworkServiceCreateRequest) error {
	// id must not be nil and must not conflict
	// if partition is specified, only one per partition is possible, otherwise only one without partition
	// if this is project scoped, child project must match, otherwise can be freely specified.
	// If the vrf id is given, child networks will inherit this vrf.
	// If the vrf id is nil in this network, child vrf is taken from the pool.
	// prefixes must be specified, default- and min childprefixlength must match prefix addressfamilies
	// parent network id must not be specified
	// additionalannouncable cidrs should be specified and validated
	// nat must not be specified
	// addressfamily and length must not be specified

	var errs []error

	errs = validate(errs, req.Id != nil, "id must not be nil")
	errs = validate(errs, req.Prefixes != nil, "prefixes must not be nil")
	errs = validate(errs, req.DefaultChildPrefixLength != nil, "default child prefix length must not be nil")

	errs = validate(errs, req.ParentNetworkId == nil, "parent network id must be nil")
	errs = validate(errs, req.AddressFamily == nil, "addressfamily must be nil")
	errs = validate(errs, req.Length == nil, "length must be nil")

	if len(errs) > 0 {
		return errorutil.NewInvalidArgument(errors.Join(errs...))
	}

	if req.Partition != nil && req.Type == apiv2.NetworkType_NETWORK_TYPE_SUPER_NAMESPACED {
		return errorutil.InvalidArgument("partition must not be specified for namespaced private super")
	}
	if err := r.networkTypeInPartitionPossible(ctx, req.Partition, &req.Type); err != nil {
		return err
	}

	prefixes, err := metal.NewPrefixesFromCIDRs(req.Prefixes)
	if err != nil {
		return errorutil.NewInvalidArgument(err)
	}

	defaultChildPrefixLength, err := metal.ToChildPrefixLength(req.DefaultChildPrefixLength, prefixes)
	if err != nil {
		return errorutil.NewInvalidArgument(err)
	}

	if _, err := metal.ToChildPrefixLength(req.MinChildPrefixLength, prefixes); err != nil {
		return errorutil.NewInvalidArgument(err)
	}

	if err := r.validatePrefixesAndAddressFamilies(prefixes, nil, defaultChildPrefixLength, true); err != nil {
		return errorutil.NewInvalidArgument(err)
	}

	if err := r.validateAdditionalAnnouncableCIDRs(req.AdditionalAnnouncableCidrs, true); err != nil {
		return errorutil.NewInvalidArgument(err)

	}

	if err := r.prefixesOverlapping(ctx, prefixes.String()); err != nil {
		return err
	}

	return nil
}

func (r *networkRepository) validateCreateNetworkTypeExternal(ctx context.Context, req *adminv2.NetworkServiceCreateRequest) error {
	// id must not be nil and must not conflict
	// partition is optional, multiple external per partition are possible
	// project is optional (if given, only ips with this project can be acquired in this network)
	// vrf must not be nil
	// prefixes must be specified
	// destination prefixes can be specified
	// default- and min childprefixlength must not be specified
	// parent network id must not be specified
	// additionalannouncable cidrs must not be specified
	// nat is optional
	// addressfamily and length must not be specified

	var errs []error

	errs = validate(errs, req.Id != nil, "id must not be nil")
	errs = validate(errs, req.Prefixes != nil, "prefixes must not be nil")
	errs = validate(errs, req.Vrf != nil, "vrf must not be nil")

	errs = validate(errs, req.ParentNetworkId == nil, "parent network id must be nil")
	errs = validate(errs, req.AddressFamily == nil, "addressfamily must be nil")
	errs = validate(errs, req.Length == nil, "length must be nil")
	errs = validate(errs, req.AdditionalAnnouncableCidrs == nil, "additional announcable cidrs must be nil")
	errs = validate(errs, req.DefaultChildPrefixLength == nil, "default child prefix length must be nil")
	errs = validate(errs, req.MinChildPrefixLength == nil, "min child prefix length must be nil")

	if len(errs) > 0 {
		return errorutil.NewInvalidArgument(errors.Join(errs...))
	}

	prefixes, err := metal.NewPrefixesFromCIDRs(req.Prefixes)
	if err != nil {
		return errorutil.NewInvalidArgument(err)
	}
	destinationprefixes, err := metal.NewPrefixesFromCIDRs(req.DestinationPrefixes)
	if err != nil {
		return errorutil.NewInvalidArgument(err)
	}

	if err := r.validatePrefixesAndAddressFamilies(prefixes, destinationprefixes.AddressFamilies(), nil, false); err != nil {
		return errorutil.NewInvalidArgument(err)
	}

	if err := r.prefixesOverlapping(ctx, prefixes.String()); err != nil {
		return err
	}
	return nil
}

func (r *networkRepository) validateCreateNetworkTypeUnderlay(ctx context.Context, req *adminv2.NetworkServiceCreateRequest) error {
	// id must not be nil and must not conflict
	// partition must be specified
	// project must be nil
	// vrf must be nil
	// prefixes must be specified, ipv4 only
	// destination prefixes must be empty
	// default- and min childprefixlength must not be specified
	// parent network id must not be specified
	// additionalannouncable cidrs must not be specified
	// nat must be none
	// addressfamily and length must not be specified

	var errs []error

	errs = validate(errs, req.Id != nil, "id must not be nil")
	errs = validate(errs, req.Prefixes != nil, "prefixes must not be nil")
	errs = validate(errs, req.Partition != nil, "partition must not be nil")

	errs = validate(errs, req.Project == nil, "project must be nil")
	errs = validate(errs, req.Vrf == nil, "vrf must be nil")
	errs = validate(errs, req.ParentNetworkId == nil, "parent network id must be nil")
	errs = validate(errs, req.AddressFamily == nil, "addressfamily must be nil")
	errs = validate(errs, req.Length == nil, "length must be nil")
	errs = validate(errs, req.DestinationPrefixes == nil, "destination prefixes must be nil")
	errs = validate(errs, req.AdditionalAnnouncableCidrs == nil, "additional announcable cidrs must be nil")
	errs = validate(errs, req.DefaultChildPrefixLength == nil, "default child prefix length must be nil")
	errs = validate(errs, req.MinChildPrefixLength == nil, "min child prefix length must be nil")

	if len(errs) > 0 {
		return errorutil.NewInvalidArgument(errors.Join(errs...))
	}

	if req.NatType != nil && *req.NatType != apiv2.NATType_NAT_TYPE_NONE {
		return errorutil.InvalidArgument("nattype my only be nil or none")
	}

	prefixes, err := metal.NewPrefixesFromCIDRs(req.Prefixes)
	if err != nil {
		return errorutil.NewInvalidArgument(err)
	}
	if slices.Contains(prefixes.AddressFamilies(), metal.IPv6AddressFamily) {
		return errorutil.InvalidArgument("underlay can only contain ipv4 prefixes")
	}

	// theoretically underlay networks can overlap by partition, but if an operator wants to do zonal routing
	// this would be forbidden, so we restrict this for now
	if err := r.prefixesOverlapping(ctx, prefixes.String()); err != nil {
		return err
	}

	return nil
}

func (r *networkRepository) prefixesOverlapping(ctx context.Context, prefixes []string) error {
	allNetworks, err := r.List(ctx, &apiv2.NetworkQuery{})
	if err != nil {
		return errorutil.Convert(err)
	}

	var (
		existingPrefixes    = metal.Prefixes{}
		existingPrefixesMap = make(map[string]bool)
	)

	for _, nw := range allNetworks {
		if nw.ParentNetworkID != "" {
			// as we check the super networks this includes the child networks automatically
			// theoretically it would be nice to filter them out directly in the database query
			continue
		}
		if nw.Namespace != nil {
			// super namespaced networks can overlap!
			continue
		}

		for _, p := range nw.Prefixes {
			_, ok := existingPrefixesMap[p.String()]
			if !ok {
				existingPrefixes = append(existingPrefixes, p)
				existingPrefixesMap[p.String()] = true
			}
		}
	}

	err = goipam.PrefixesOverlapping(existingPrefixes.String(), prefixes)
	if err != nil {
		return errorutil.NewConflict(err)
	}

	return nil
}

func (r *networkRepository) networkTypeInPartitionPossible(ctx context.Context, partition *string, networkType *apiv2.NetworkType) error {
	_, err := r.Find(ctx, &apiv2.NetworkQuery{Partition: partition, Type: networkType})
	if !errorutil.IsNotFound(err) {
		return errorutil.InvalidArgument("partition with id %q already has a network of type %s", pointer.SafeDeref(partition), networkType)
	}
	return nil
}

func (r *networkRepository) validatePrefixesAndAddressFamilies(prefixes metal.Prefixes, destPrefixesAfs metal.AddressFamilies, defaultChildPrefixLength metal.ChildPrefixLength, super bool) error {
	for _, af := range destPrefixesAfs {
		if !slices.Contains(prefixes.AddressFamilies(), af) {
			return fmt.Errorf("addressfamily:%s of destination prefixes is not present in existing prefixes", af)
		}
	}

	if !super {
		return nil
	}

	if len(defaultChildPrefixLength) == 0 {
		return fmt.Errorf("a super network must always contain a default child prefix length")
	}

	for _, af := range prefixes.AddressFamilies() {
		if _, exists := defaultChildPrefixLength[af]; !exists {
			return fmt.Errorf("a super network must always contain a default child prefix length per addressfamily: %s", af)
		}
	}

	return nil
}

func (r *networkRepository) validateAdditionalAnnouncableCIDRs(additionalCidrs []string, super bool) error {
	if len(additionalCidrs) == 0 {
		return nil
	}

	if !super {
		return errors.New("additional announcable cidrs can only be set in a private super network")
	}

	for _, cidr := range additionalCidrs {
		_, err := netip.ParsePrefix(cidr)
		if err != nil {
			return fmt.Errorf("given cidr:%q in additional announcable cidrs is malformed:%w", cidr, err)
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
		return nil, errorutil.InvalidArgument("default child prefix length can only be set on super networks")
	}

	if old.ParentNetworkID != "" || (old.NetworkType != nil && *old.NetworkType == metal.ChildNetworkType) {
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

func (r *networkRepository) arePrefixesEmpty(ctx context.Context, prefixes metal.Prefixes) error {
	for _, prefixToCheck := range prefixes {
		ips, err := r.r.UnscopedIP().List(ctx, &apiv2.IPQuery{ParentPrefixCidr: pointer.Pointer(prefixToCheck.String())})
		if err != nil {
			return errorutil.Convert(err)
		}
		if len(ips) > 0 {
			return errorutil.InvalidArgument("there are still %d ips present in prefix: %s", len(ips), prefixToCheck)
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
