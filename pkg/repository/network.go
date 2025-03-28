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
			return nil, errorutil.Convert(err)
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
		return nil, errorutil.Convert(err)
	}
	destPrefixes, err := metal.NewPrefixesFromCIDRs(req.DestinationPrefixes)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	err = validatePrefixesAndAddressFamilies(prefixes, destPrefixes.AddressFamilies(), childPrefixLength, privateSuper)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	err = validateAdditionalAnnouncableCIDRs(req.AdditionalAnnounceableCidrs, privateSuper)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	allNetworks, err := r.List(ctx, &adminv2.NetworkServiceListRequest{})
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
		return nil, errorutil.Convert(err)
	}

	if req.Partition != nil {
		partition, err := r.r.Partition().Get(ctx, *req.Partition)
		if err != nil {
			return nil, errorutil.Convert(err)
		}
		var nw *metal.Network
		if privateSuper {
			nw, err = r.Find(ctx, &adminv2.NetworkServiceListRequest{Query: &apiv2.NetworkQuery{Partition: req.Partition, Options: &apiv2.NetworkQuery_Options{PrivateSuper: &privateSuper}}})
			if err != nil && !errorutil.IsNotFound(err) {
				return nil, errorutil.Convert(err)
			}
			if nw.ID != "" {
				return nil, errorutil.InvalidArgument("partition with id %q already has a private super network", partition.ID)
			}
		}

		if underlay {
			_, err = r.Find(ctx, &adminv2.NetworkServiceListRequest{Query: &apiv2.NetworkQuery{Partition: req.Partition, Options: &apiv2.NetworkQuery_Options{Underlay: &underlay}}})
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
				return fmt.Errorf("given prefix %v is not a valid ip with mask: %w", p, err)
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

	children, err := r.List(ctx, &adminv2.NetworkServiceListRequest{Query: &apiv2.NetworkQuery{ParentNetworkId: &req.ID}})
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
	if err != nil && !errorutil.IsNotFound(err) {
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
	if r.scope.projectID == nw.ProjectID {
		return nil
	}
	return errorutil.NotFound("network:%s project:%s for scope:%s not found", nw.ID, nw.ProjectID, r.scope.projectID)
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
	r.log.Info("delete network", "uuid", payload.UUID)

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
		err = r.ds.VrfPool().ReleaseUniqueInteger(nw.Vrf)
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

func (r *networkRepository) AllocateNetwork(ctx context.Context, rq *apiv2.NetworkServiceCreateRequest) (*metal.Network, error) {
	panic("unimplemented")
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

		privateSuper bool
		underlay     bool
		nat          bool
		vrfShared    bool

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

	var vrf uint

	if req.Vrf != nil {
		vrf, err = r.r.ds.VrfPool().AcquireUniqueInteger(uint(*req.Vrf))
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

	if new.Name != nil {
		newNetwork.Name = *new.Name
	}
	if new.Description != nil {
		newNetwork.Description = *new.Description
	}
	if new.Meta.Labels != nil {
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

	err = r.r.ds.Network().Update(ctx, &newNetwork, old)
	if err != nil {
		return nil, err
	}

	return &newNetwork, nil
}

func (r *networkRepository) Find(ctx context.Context, query *adminv2.NetworkServiceListRequest) (*metal.Network, error) {
	nw, err := r.r.ds.Network().Find(ctx, r.scopedNetworkFilters(queries.NetworkFilter(query.Query))...)
	if err != nil {
		return nil, err
	}

	return nw, nil
}
func (r *networkRepository) List(ctx context.Context, query *adminv2.NetworkServiceListRequest) ([]*metal.Network, error) {
	nws, err := r.r.ds.Network().List(ctx, r.scopedNetworkFilters(queries.NetworkFilter(query.Query))...)
	if err != nil {
		return nil, err
	}

	return nws, nil
}
func (r *networkRepository) ConvertToInternal(msg *apiv2.Network) (*metal.Network, error) {
	panic("unimplemented")
}
func (r *networkRepository) ConvertToProto(e *metal.Network) (*apiv2.Network, error) {
	panic("unimplemented")
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
	r.r.log.Info("scopedFilters", "scope", r.scope)
	if r.scope != nil {
		qs = append(qs, queries.NetworkProjectScoped(r.scope.projectID))
	}
	if filter != nil {
		qs = append(qs, filter)
	}
	return qs
}
