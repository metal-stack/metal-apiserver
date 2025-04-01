package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"

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

	if len(req.Prefixes) == 0 {
		return nil, errorutil.InvalidArgument("no prefixes given")
	}

	var (
		privateSuper bool
		underlay     bool
		nat          bool
	)
	if req.Options != nil {
		privateSuper = req.Options.PrivateSuper
		underlay = req.Options.Underlay
		nat = req.Options.Nat
	}

	if !privateSuper && (req.DefaultChildPrefixLength != nil || len(req.DefaultChildPrefixLength) > 0) {
		return nil, errorutil.InvalidArgument("defaultchildprefixlength can only be set for privatesuper networks")
	}

	var childPrefixLength = metal.ChildPrefixLength{}
	for _, pl := range req.DefaultChildPrefixLength {
		af, err := metal.ToAddressFamily(pl.AddressFamily)
		if err != nil {
			return nil, errorutil.NewInvalidArgument(err)
		}
		childPrefixLength[af] = uint8(pl.Length)
	}

	prefixes, err := metal.NewPrefixesFromCIDRs(req.Prefixes)
	if err != nil {
		return nil, errorutil.NewInvalidArgument(err)
	}
	destPrefixes, err := metal.NewPrefixesFromCIDRs(req.DestinationPrefixes)
	if err != nil {
		return nil, errorutil.NewInvalidArgument(err)
	}

	err = validatePrefixesAndAddressFamilies(prefixes, destPrefixes.AddressFamilies(), childPrefixLength, privateSuper)
	if err != nil {
		return nil, errorutil.NewInvalidArgument(err)
	}

	err = validateAdditionalAnnouncableCIDRs(req.AdditionalAnnounceableCidrs, privateSuper)
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

	if req.Partition != nil {
		partition, err := r.r.Partition().Get(ctx, *req.Partition)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		if privateSuper {
			_, err = r.Find(ctx, &apiv2.NetworkQuery{Partition: req.Partition, Options: &apiv2.NetworkQuery_Options{PrivateSuper: &privateSuper}})
			if err != nil && !errorutil.IsNotFound(err) {
				return nil, errorutil.Convert(err)
			}
			if !errorutil.IsNotFound(err) {
				return nil, errorutil.InvalidArgument("partition with id %q already has a private super network", partition.ID)
			}
		}

		if underlay {
			_, err = r.Find(ctx, &apiv2.NetworkQuery{Partition: req.Partition, Options: &apiv2.NetworkQuery_Options{Underlay: &underlay}})
			if err != nil && !errorutil.IsNotFound(err) {
				return nil, errorutil.Convert(err)
			} else {
				return nil, errorutil.InvalidArgument("partition with id %q already has an underlay network", partition.ID)
			}
		}
	}

	if (privateSuper || underlay) && nat {
		return nil, errorutil.InvalidArgument("private super or underlay network is not supposed to NAT")
	}

	return &Validated[*adminv2.NetworkServiceCreateRequest]{
		message: req,
	}, nil
}

func validatePrefixesAndAddressFamilies(prefixes metal.Prefixes, destPrefixesAfs metal.AddressFamilies, defaultChildPrefixLength metal.ChildPrefixLength, privateSuper bool) error {

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

	for af, length := range defaultChildPrefixLength {
		// check if childprefixlength is set and matches addressfamily
		for _, p := range prefixes.OfFamily(af) {
			ipprefix, err := netip.ParsePrefix(p.String())
			if err != nil {
				return fmt.Errorf("given prefix %q is not a valid ip with mask: %w", p.String(), err)
			}
			if int(length) <= ipprefix.Bits() {
				return fmt.Errorf("given defaultchildprefixlength %d is not greater than prefix length of:%s", length, p.String())
			}
		}
	}

	for _, af := range prefixes.AddressFamilies() {
		if _, exists := defaultChildPrefixLength[af]; !exists {
			return fmt.Errorf("private super network must always contain a defaultchildprefixlength per addressfamily:%s", af)
		}
	}

	return nil
}

func validateAdditionalAnnouncableCIDRs(additionalCidrs []string, privateSuper bool) error {
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
	old, err := r.Get(ctx, req.Network.Id)
	if err != nil {
		return nil, err
	}
	newNetwork := *old

	new := req.Network

	if old.Shared && !new.Options.Shared {
		return nil, errorutil.InvalidArgument("once a network is marked as shared it is not possible to unshare it")
	}
	if len(new.DefaultChildPrefixLength) > 0 && !old.PrivateSuper {
		return nil, errorutil.InvalidArgument("defaultchildprefixlength can only be set on privatesuper")
	}

	var (
		prefixesToBeRemoved metal.Prefixes
		prefixesToBeAdded   metal.Prefixes
	)

	if len(new.Prefixes) > 0 {
		prefixes, err := metal.NewPrefixesFromCIDRs(new.Prefixes)
		if err != nil {
			return nil, errorutil.Convert(err)
		}

		newNetwork.Prefixes = prefixes

		prefixesToBeRemoved = old.SubtractPrefixes(prefixes...)

		for _, prefixToBeRemoved := range prefixesToBeRemoved {
			ips, err := r.r.UnscopedIP().List(ctx, &apiv2.IPQuery{ParentPrefixCidr: pointer.Pointer(prefixToBeRemoved.String())})
			if err != nil {
				return nil, errorutil.Convert(err)
			}
			if len(ips) > 0 {
				return nil, errorutil.InvalidArgument("there are still ips:%v present in one of the prefixes which should be removed:%s", ips, prefixToBeRemoved)
			}
		}

		prefixesToBeAdded = newNetwork.SubtractPrefixes(old.Prefixes...)
	}

	var (
		destPrefixAfs metal.AddressFamilies
	)
	if len(new.DestinationPrefixes) > 0 {
		destPrefixes, err := metal.NewPrefixesFromCIDRs(new.DestinationPrefixes)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		newNetwork.DestinationPrefixes = destPrefixes
		destPrefixAfs = destPrefixes.AddressFamilies()
	}

	err = validatePrefixesAndAddressFamilies(newNetwork.Prefixes, destPrefixAfs, old.DefaultChildPrefixLength, old.PrivateSuper)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	if len(new.DefaultChildPrefixLength) > 0 {
		newDefaultChildPrefixLength, err := metal.ToDefaultChildPrefixLength(new.DefaultChildPrefixLength, newNetwork.Prefixes)
		if err != nil {
			return nil, errorutil.NewInvalidArgument(err)
		}
		newNetwork.DefaultChildPrefixLength = newDefaultChildPrefixLength
	}

	err = validateAdditionalAnnouncableCIDRs(new.AdditionalAnnouncebleCidrs, old.PrivateSuper)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	for _, oldcidr := range old.AdditionalAnnouncableCIDRs {
		if !req.Force && !slices.Contains(new.AdditionalAnnouncebleCidrs, oldcidr) {
			return nil, errorutil.InvalidArgument("you cannot remove %q from additionalannouncablecidrs without force flag set", oldcidr)
		}
	}

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

	return &Validated[*adminv2.NetworkServiceUpdateRequest]{
		message: req,
	}, nil
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

	for _, prefix := range old.Prefixes {
		ips, err := r.r.UnscopedIP().List(ctx, &apiv2.IPQuery{ParentPrefixCidr: pointer.Pointer(prefix.String())})
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		if len(ips) > 0 {
			return nil, errorutil.InvalidArgument("there are still ips:%v present in one of the prefixes:%s", ips, prefix)
		}
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

	if rq.Options != nil {
		if rq.Options.Underlay {
			return nil, errorutil.InvalidArgument("it is not possible to allocate a underlay network")
		}
		if rq.Options.PrivateSuper {
			return nil, errorutil.InvalidArgument("it is not possible to allocate a private-super network")
		}
		if rq.Options.Shared {
			return nil, errorutil.InvalidArgument("it is not possible to allocate a shared network")
		}
	}

	destPrefixes := metal.Prefixes{}
	for _, p := range rq.DestinationPrefixes {
		prefix, _, err := metal.NewPrefixFromCIDR(p)
		if err != nil {
			return nil, errorutil.InvalidArgument("given prefix %v is not a valid ip with mask: %w", p, err)

		}

		destPrefixes = append(destPrefixes, *prefix)
	}

	var superNetwork *metal.Network
	if rq.ParentNetworkId != nil {
		superNetwork, err = r.r.UnscopedNetwork().Get(ctx, *rq.ParentNetworkId)
		if err != nil {
			return nil, err
		}
	} else {
		superNetwork, err = r.r.UnscopedNetwork().Find(ctx, &apiv2.NetworkQuery{
			Partition: &partition.ID,
			Options: &apiv2.NetworkQuery_Options{
				PrivateSuper: pointer.Pointer(true),
			},
		})
		if err != nil {
			return nil, errorutil.InvalidArgument("unable to find a privatesuper in partition:%s %w", partition.ID, err)
		}
	}

	if len(superNetwork.DefaultChildPrefixLength) == 0 {
		return nil, errorutil.InvalidArgument("supernetwork %s has no defaultchildprefixlength specified", superNetwork.ID)
	}

	length := superNetwork.DefaultChildPrefixLength
	if len(rq.Length) > 0 {
		for _, cpl := range rq.Length {
			addressfamily, err := metal.ToAddressFamily(cpl.AddressFamily)
			if err != nil {
				return nil, errorutil.InvalidArgument("addressfamily of length is invalid %w", err)
			}
			length[addressfamily] = uint8(cpl.Length)
		}
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

	err = validatePrefixesAndAddressFamilies(superNetwork.Prefixes, destPrefixes.AddressFamilies(), length, false)
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

		destPrefixes  = metal.Prefixes{}
		childPrefixes = metal.Prefixes{}

		nat    bool
		shared bool
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
	if req.Options != nil {
		nat = req.Options.Nat
		shared = req.Options.Shared
	}

	parent, err := r.r.UnscopedNetwork().Find(ctx, &apiv2.NetworkQuery{
		Partition: &partition,
		Options: &apiv2.NetworkQuery_Options{
			PrivateSuper: pointer.Pointer(true),
		},
	})
	if err != nil {
		return nil, errorutil.InvalidArgument("unable to find a privatesuper in partition:%s %w", partition, err)
	}

	vrf, err := r.r.ds.VrfPool().AcquireRandomUniqueInteger(ctx)
	if err != nil {
		return nil, errorutil.Internal("could not acquire a vrf: %w", err)
	}

	for _, p := range req.DestinationPrefixes {
		prefix, _, err := metal.NewPrefixFromCIDR(p)
		if err != nil {
			return nil, errorutil.InvalidArgument("given prefix %v is not a valid ip with mask: %w", p, err)
		}

		destPrefixes = append(destPrefixes, *prefix)
	}
	length := parent.DefaultChildPrefixLength

	if len(req.Length) > 0 {
		for _, pl := range req.Length {
			af, err := metal.ToAddressFamily(pl.AddressFamily)
			if err != nil {
				return nil, errorutil.NewInvalidArgument(err)
			}
			length[af] = uint8(pl.Length)
		}
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
		Prefixes:            childPrefixes,
		DestinationPrefixes: destPrefixes,
		PartitionID:         partition,
		ProjectID:           req.Project,
		Nat:                 nat,
		PrivateSuper:        false,
		Underlay:            false,
		Shared:              shared,
		Vrf:                 vrf,
		ParentNetworkID:     parent.ID,
		Labels:              labels,
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

		privateSuper bool
		underlay     bool
		nat          bool
		vrfShared    bool
		shared       bool

		childPrefixLength = metal.ChildPrefixLength{}
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

	if req.Options != nil {
		privateSuper = req.Options.PrivateSuper
		underlay = req.Options.Underlay
		nat = req.Options.Nat
		vrfShared = req.Options.VrfShared
		shared = req.Options.Shared
	}

	prefixes, err := metal.NewPrefixesFromCIDRs(req.Prefixes)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	destPrefixes, err := metal.NewPrefixesFromCIDRs(req.DestinationPrefixes)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	for _, pl := range req.DefaultChildPrefixLength {
		af, err := metal.ToAddressFamily(pl.AddressFamily)
		if err != nil {
			return nil, errorutil.NewInvalidArgument(err)
		}
		childPrefixLength[af] = uint8(pl.Length)
	}

	if req.Vrf != nil {
		vrf, err = r.r.ds.VrfPool().AcquireUniqueInteger(ctx, uint(*req.Vrf))
		if err != nil {
			if !errorutil.IsConflict(err) {
				return nil, errorutil.InvalidArgument("could not acquire vrf: %w", err)
			}
			if !vrfShared {
				return nil, errorutil.InvalidArgument("cannot acquire a unique vrf id twice except vrfShared is set to true: %w", err)
			}
		}
	} else {
		// FIXME in case req.vrf is nil, the network will be created with a 0 vrf ? This is the case in the actual metal-api implementation
	}

	nw := &metal.Network{
		Base: metal.Base{
			ID:          id,
			Name:        name,
			Description: description,
		},
		Prefixes:                   prefixes,
		DestinationPrefixes:        destPrefixes,
		DefaultChildPrefixLength:   childPrefixLength,
		PartitionID:                partition,
		ProjectID:                  projectId,
		Nat:                        nat,
		PrivateSuper:               privateSuper,
		Underlay:                   underlay,
		Vrf:                        vrf,
		Shared:                     shared,
		Labels:                     labels,
		AdditionalAnnouncableCIDRs: req.AdditionalAnnounceableCidrs,
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

func (r *networkRepository) Update(ctx context.Context, req *Validated[*adminv2.NetworkServiceUpdateRequest]) (*metal.Network, error) {
	old, err := r.Get(ctx, req.message.Network.Id)
	if err != nil {
		return nil, err
	}
	newNetwork := *old

	new := req.message.Network
	r.r.log.Debug("update network", "old", old, "new", new)

	if new.Name != nil {
		newNetwork.Name = *new.Name
	}
	if new.Description != nil {
		newNetwork.Description = *new.Description
	}
	if new.Meta != nil && new.Meta.Labels != nil {
		newNetwork.Labels = new.Meta.Labels.Labels
	}
	// Fixme network options
	// if new.Options != nil && new.Options.Shared != nil {
	// 	newNetwork.Shared = *new.Shared
	// }

	var (
		prefixesToBeRemoved metal.Prefixes
		prefixesToBeAdded   metal.Prefixes
	)

	if len(new.Prefixes) > 0 {
		prefixes, err := metal.NewPrefixesFromCIDRs(new.Prefixes)
		if err != nil {
			return nil, errorutil.Convert(err)
		}

		newNetwork.Prefixes = prefixes
		prefixesToBeRemoved = old.SubtractPrefixes(prefixes...)
		prefixesToBeAdded = newNetwork.SubtractPrefixes(old.Prefixes...)
	}

	if len(new.DestinationPrefixes) > 0 {
		destPrefixes, err := metal.NewPrefixesFromCIDRs(new.DestinationPrefixes)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		newNetwork.DestinationPrefixes = destPrefixes
	}

	if len(new.DefaultChildPrefixLength) > 0 {
		newDefaultChildPrefixLength, err := metal.ToDefaultChildPrefixLength(new.DefaultChildPrefixLength, newNetwork.Prefixes)
		if err != nil {
			return nil, errorutil.NewInvalidArgument(err)
		}
		newNetwork.DefaultChildPrefixLength = newDefaultChildPrefixLength
	}

	newNetwork.AdditionalAnnouncableCIDRs = new.AdditionalAnnouncebleCidrs

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
	err = r.r.ds.Network().Update(ctx, &newNetwork, old)
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

	return nws, nil
}
func (r *networkRepository) ConvertToInternal(msg *apiv2.Network) (*metal.Network, error) {
	panic("unimplemented")
}
func (r *networkRepository) ConvertToProto(e *metal.Network) (*apiv2.Network, error) {
	var (
		defaultChildPrefixLength []*apiv2.ChildPrefixLength
		consumption              *apiv2.NetworkConsumption
		options                  *apiv2.NetworkOptions
		labels                   *apiv2.Labels
	)

	if e == nil {
		return nil, nil
	}

	if e.Labels != nil {
		labels = &apiv2.Labels{
			Labels: e.Labels,
		}
	}

	if e.Nat || e.PrivateSuper || e.Shared || e.Underlay {
		options = &apiv2.NetworkOptions{
			Shared:       e.Underlay,
			Nat:          e.Nat,
			PrivateSuper: e.PrivateSuper,
			Underlay:     e.Underlay,
		}
	}

	for af, length := range e.DefaultChildPrefixLength {
		var newAF apiv2.IPAddressFamily
		switch af {
		case metal.IPv4AddressFamily:
			newAF = apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4
		case metal.IPv6AddressFamily:
			newAF = apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6
		default:
			return nil, errorutil.InvalidArgument("unknown addressfamily %s", af)
		}
		defaultChildPrefixLength = append(defaultChildPrefixLength, &apiv2.ChildPrefixLength{
			AddressFamily: newAF,
			Length:        uint32(length),
		})
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
		AdditionalAnnouncebleCidrs: e.AdditionalAnnouncableCIDRs,
		Meta: &apiv2.Meta{
			Labels:    labels,
			CreatedAt: timestamppb.New(e.Created),
			UpdatedAt: timestamppb.New(e.Changed),
		},
		Options:                  options,
		DefaultChildPrefixLength: defaultChildPrefixLength,
	}
	consumption, err := r.GetNetworkUsage(context.Background(), e)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	nw.Consumption = consumption

	return nw, nil
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
