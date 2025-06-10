package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamapiv1 "github.com/metal-stack/go-ipam/api/v1"
	asyncclient "github.com/metal-stack/metal-apiserver/pkg/async/client"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/metal-stack/metal-lib/pkg/tag"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type (
	ipRepository struct {
		s     *Store
		scope *ProjectScope
	}
)

func (r *ipRepository) get(ctx context.Context, id string) (*metal.IP, error) {
	ip, err := r.s.ds.IP().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return ip, nil
}

func (r *ipRepository) matchScope(ip *metal.IP) bool {
	if r.scope == nil {
		return true
	}

	return r.scope.projectID == pointer.SafeDeref(ip).ProjectID
}

func (r *ipRepository) create(ctx context.Context, req *apiv2.IPServiceCreateRequest) (*metal.IP, error) {
	var (
		name        string
		description string
	)

	if req.Name != nil {
		name = *req.Name
	}
	if req.Description != nil {
		description = *req.Description
	}

	var tags []string
	if req.Labels != nil {
		tags = tag.TagMap(req.Labels.Labels).Slice()
	}

	if req.MachineId != nil {
		tags = append(tags, tag.New(tag.MachineID, *req.MachineId))
	}
	// Ensure no duplicates
	tags = tag.NewTagMap(tags).Slice()

	nw, err := r.s.UnscopedNetwork().Get(ctx, req.Network) // TODO: maybe it would be useful to be able to pass this through from the validation or use a short-lived cache in the ip repo
	if err != nil {
		return nil, err
	}

	if nw.ProjectID != "" && nw.ProjectID != req.Project {
		return nil, errorutil.InvalidArgument("not allowed to create ip with project %s in network %s scoped to project %s", req.Project, req.Network, nw.ProjectID)
	}

	// for private, unshared networks the project id must be the same
	// for external networks the project id is not checked
	// if !nw.Shared && nw.ParentNetworkID != "" && p.Meta.Id != nw.ProjectID {
	if nw.ProjectID != req.Project {
		switch *nw.NetworkType {
		case metal.NetworkTypeChildShared, metal.NetworkTypeExternal:
			// this is fine
		default:
			return nil, errorutil.InvalidArgument("can not allocate ip for project %q because network belongs to %q and the network is of type:%s", req.Project, nw.ProjectID, *nw.NetworkType)
		}
	}

	// FIXME: move validation to ip validation

	var af *metal.AddressFamily
	if req.AddressFamily != nil {
		convertedAf, err := metal.ToAddressFamily(*req.AddressFamily)
		if err != nil {
			return nil, errorutil.NewInvalidArgument(err)
		}

		if !slices.Contains(nw.Prefixes.AddressFamilies(), convertedAf) {
			return nil, errorutil.InvalidArgument("there is no prefix for the given addressfamily:%s present in network:%s %s", convertedAf, req.Network, nw.Prefixes.AddressFamilies())
		}
		if req.Ip != nil {
			return nil, errorutil.InvalidArgument("it is not possible to specify specificIP and addressfamily")
		}
		af = &convertedAf
	}

	var (
		ipAddress    string
		ipParentCidr string
	)

	// as this is more or less a transaction... shouldn't we put this into async?

	if req.Ip == nil {
		ipAddress, ipParentCidr, err = r.allocateRandomIP(ctx, nw, af)
		if err != nil {
			return nil, err
		}
	} else {
		ipAddress, ipParentCidr, err = r.allocateSpecificIP(ctx, nw, *req.Ip)
		if err != nil {
			return nil, err
		}
	}

	ipType, err := metal.ToIPType(req.Type)
	if err != nil {
		return nil, err
	}

	uuid, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	ip := &metal.IP{
		AllocationUUID:   uuid.String(),
		IPAddress:        metal.CreateNamespacedIPAddress(nw.Namespace, ipAddress),
		Namespace:        nw.Namespace,
		ParentPrefixCidr: ipParentCidr,
		Name:             name,
		Description:      description,
		NetworkID:        nw.ID,
		ProjectID:        req.Project,
		Type:             ipType,
		Tags:             tags,
	}

	resp, err := r.s.ds.IP().Create(ctx, ip)
	if err != nil {
		return nil, err
	}

	r.s.log.Info("created ip in metal-db", "ip", ipAddress, "network", nw.ID, "type", ipType)

	return resp, nil
}

func (r *ipRepository) update(ctx context.Context, e *metal.IP, req *apiv2.IPServiceUpdateRequest) (*metal.IP, error) {
	rq := req
	old, err := r.get(ctx, rq.Ip)
	if err != nil {
		return nil, err
	}

	new := *old

	if rq.Description != nil {
		new.Description = *rq.Description
	}
	if rq.Name != nil {
		new.Name = *rq.Name
	}
	if rq.Type != nil {
		var t metal.IPType
		switch rq.Type.String() {
		case apiv2.IPType_IP_TYPE_EPHEMERAL.String():
			t = metal.Ephemeral
		case apiv2.IPType_IP_TYPE_STATIC.String():
			t = metal.Static
		case apiv2.IPType_IP_TYPE_UNSPECIFIED.String():
			return nil, errorutil.InvalidArgument("ip type cannot be unspecified: %s", rq.Type)
		}
		new.Type = t
	}
	if rq.Labels != nil {
		new.Tags = updateLabelsOnSlice(rq.Labels, new.Tags)
	}

	err = r.s.ds.IP().Update(ctx, &new)
	if err != nil {
		return nil, err
	}

	return &new, nil
}

func (r *ipRepository) delete(ctx context.Context, e *metal.IP) error {
	info, err := r.s.async.NewIPDeleteTask(e.AllocationUUID, e.IPAddress, e.ProjectID)
	if err != nil {
		return err
	}

	r.s.log.Info("ip delete queued", "info", info)

	return nil
}

func (r *ipRepository) find(ctx context.Context, rq *apiv2.IPQuery) (*metal.IP, error) {
	ip, err := r.s.ds.IP().Find(ctx, r.scopedIPFilters(queries.IpFilter(rq))...)
	if err != nil {
		return nil, err
	}

	return ip, nil
}

func (r *ipRepository) list(ctx context.Context, rq *apiv2.IPQuery) ([]*metal.IP, error) {
	ip, err := r.s.ds.IP().List(ctx, r.scopedIPFilters(queries.IpFilter(rq))...)
	if err != nil {
		return nil, err
	}

	return ip, nil
}

func (r *ipRepository) allocateSpecificIP(ctx context.Context, parent *metal.Network, specificIP string) (ipAddress, parentPrefixCidr string, err error) {
	parsedIP, err := netip.ParseAddr(specificIP)
	if err != nil {
		return "", "", fmt.Errorf("unable to parse specific ip: %w", err)
	}

	af := metal.AddressFamilyIPv4
	if parsedIP.Is6() {
		af = metal.AddressFamilyIPv6
	}

	for _, prefix := range parent.Prefixes.OfFamily(af) {
		pfx, err := netip.ParsePrefix(prefix.String())
		if err != nil {
			return "", "", fmt.Errorf("unable to parse prefix: %w", err)
		}

		if !pfx.Contains(parsedIP) {
			continue
		}

		resp, err := r.s.ipam.AcquireIP(ctx, connect.NewRequest(&ipamapiv1.AcquireIPRequest{PrefixCidr: prefix.String(), Ip: &specificIP, Namespace: parent.Namespace}))
		if err != nil {
			return "", "", err
		}

		return resp.Msg.Ip.Ip, prefix.String(), nil
	}

	return "", "", errorutil.InvalidArgument("specific ip %s not contained in any of the defined prefixes", specificIP)
}

func (r *ipRepository) allocateRandomIP(ctx context.Context, parent *metal.Network, af *metal.AddressFamily) (ipAddress, parentPrefixCidr string, err error) {
	addressfamily := metal.AddressFamilyIPv4
	if af != nil {
		addressfamily = *af
	} else if len(parent.Prefixes.AddressFamilies()) == 1 {
		addressfamily = parent.Prefixes.AddressFamilies()[0]
	}

	r.s.log.Debug("allocateRandomIP from", "network", parent.ID, "addressfamily", addressfamily)
	for _, prefix := range parent.Prefixes.OfFamily(addressfamily) {
		resp, err := r.s.ipam.AcquireIP(ctx, connect.NewRequest(&ipamapiv1.AcquireIPRequest{PrefixCidr: prefix.String(), Namespace: parent.Namespace}))
		if err != nil {
			if errorutil.IsNotFound(err) {
				continue
			}
			return "", "", err
		}

		return resp.Msg.Ip.Ip, prefix.String(), nil
	}

	return "", "", errorutil.InvalidArgument("cannot allocate random free ip in ipam, no ips left in network:%s af:%s parent afs:%#v", parent.ID, addressfamily, parent.Prefixes.AddressFamilies())
}

func (r *ipRepository) convertToInternal(ip *apiv2.IP) (*metal.IP, error) {
	panic("unimplemented")
}

func (r *ipRepository) convertToProto(metalIP *metal.IP) (*apiv2.IP, error) {
	t := apiv2.IPType_IP_TYPE_UNSPECIFIED
	switch metalIP.Type {
	case metal.Ephemeral:
		t = apiv2.IPType_IP_TYPE_EPHEMERAL
	case metal.Static:
		t = apiv2.IPType_IP_TYPE_STATIC
	}

	var labels *apiv2.Labels
	if len(metalIP.Tags) > 0 {
		labels = &apiv2.Labels{
			Labels: tag.NewTagMap(metalIP.Tags),
		}
	}

	ipaddress, err := metalIP.GetIPAddress()
	if err != nil {
		return nil, err
	}

	ip := &apiv2.IP{
		Ip:          ipaddress,
		Uuid:        metalIP.AllocationUUID,
		Name:        metalIP.Name,
		Description: metalIP.Description,
		Network:     metalIP.NetworkID,
		Project:     metalIP.ProjectID,
		Namespace:   metalIP.Namespace,
		Type:        t,
		Meta: &apiv2.Meta{
			Labels:    labels,
			CreatedAt: timestamppb.New(metalIP.Created),
			UpdatedAt: timestamppb.New(metalIP.Changed),
		},
	}
	return ip, nil
}

//---------------------------------------------------------------
// Write a function HandleXXXTask to handle the input task.
// Note that it satisfies the asynq.HandlerFunc interface.
//
// Handler doesn't need to be a function. You can define a type
// that satisfies asynq.Handler interface. See examples below.
//---------------------------------------------------------------

func (r *Store) IpDeleteHandleFn(ctx context.Context, t *asynq.Task) error {

	var payload asyncclient.IPDeletePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %w %w", err, asynq.SkipRetry)
	}
	r.log.Info("delete ip", "uuid", payload.AllocationUUID, "ip", payload.IP)

	metalIP, err := r.ds.IP().Find(ctx, queries.IpFilter(&apiv2.IPQuery{Uuid: &payload.AllocationUUID}))
	if err != nil && !errorutil.IsNotFound(err) {
		return err
	}
	if metalIP == nil {
		r.log.Info("ds find, metalip is nil", "task", t)
		return nil
	}
	r.log.Info("ds find", "metalip", metalIP)

	metalNW, err := r.ds.Network().Get(ctx, metalIP.NetworkID)
	if err != nil {
		return fmt.Errorf("unable to retrieve parent network: %w", err)
	}

	_, err = r.ipam.ReleaseIP(ctx, connect.NewRequest(&ipamapiv1.ReleaseIPRequest{PrefixCidr: metalIP.ParentPrefixCidr, Ip: metalIP.IPAddress, Namespace: metalNW.Namespace}))
	if err != nil && !errorutil.IsNotFound(err) {
		r.log.Error("ipam release", "error", err)
		return err
	}

	err = r.ds.IP().Delete(ctx, metalIP)
	if err != nil && !errorutil.IsNotFound(err) {
		r.log.Error("ds delete", "error", err)
		return err
	}

	return nil
}

func (r *ipRepository) scopedIPFilters(filter generic.EntityQuery) []generic.EntityQuery {
	var qs []generic.EntityQuery
	if r.scope != nil {
		qs = append(qs, queries.IpProjectScoped(r.scope.projectID))
	}
	if filter != nil {
		qs = append(qs, filter)
	}
	return qs
}
