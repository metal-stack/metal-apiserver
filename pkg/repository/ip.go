package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strings"

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

type ipRepository struct {
	r     *Store
	scope *ProjectScope
}

func (r *ipRepository) get(ctx context.Context, id string) (*metal.IP, error) {
	ip, err := r.r.ds.IP().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return ip, nil
}

func (r *ipRepository) matchScope(ip *metal.IP) bool {
	if r.scope == nil {
		return true
	}

	if r.scope.projectID == ip.ProjectID {
		return true
	}

	return false
}

func (r *ipRepository) validateCreate(ctx context.Context, req *apiv2.IPServiceCreateRequest) error {
	if req.Network == "" {
		return fmt.Errorf("network should not be empty")
	}
	if req.Project == "" {
		return fmt.Errorf("project should not be empty")
	}

	return nil
}

func (r *ipRepository) validateUpdate(ctx context.Context, req *apiv2.IPServiceUpdateRequest, _ *metal.IP) error {
	old, err := r.find(ctx, &apiv2.IPQuery{Ip: &req.Ip, Project: &req.Project})
	if err != nil {
		return err
	}

	if req.Type != nil {
		if old.Type == metal.Static && *req.Type != apiv2.IPType_IP_TYPE_STATIC {
			return fmt.Errorf("cannot change type of ip address from static to ephemeral")
		}
	}

	return nil
}

func (r *ipRepository) validateDelete(ctx context.Context, req *metal.IP) error {
	if req.IPAddress == "" {
		return fmt.Errorf("ipaddress is empty")
	}
	if req.AllocationUUID == "" {
		return fmt.Errorf("allocationUUID is empty")
	}
	if req.ProjectID == "" {
		return fmt.Errorf("projectId is empty")
	}
	ip, err := r.find(ctx, &apiv2.IPQuery{Ip: &req.IPAddress, Uuid: &req.AllocationUUID, Project: &req.ProjectID})
	if err != nil {
		if errorutil.IsNotFound(err) {
			return nil
		}
		return err
	}

	for _, t := range ip.Tags {
		if strings.HasPrefix(t, tag.MachineID) {
			return fmt.Errorf("ip with machine scope cannot be deleted")
		}
	}

	return nil
}

func (r *ipRepository) create(ctx context.Context, rq *apiv2.IPServiceCreateRequest) (*metal.IP, error) {
	var (
		name        string
		description string
	)
	req := rq

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

	p, err := r.r.Project(req.Project).Get(ctx, req.Project)
	if err != nil {
		return nil, err
	}
	projectID := p.Meta.Id

	nw, err := r.r.Network(req.Project).Get(ctx, req.Network)
	if err != nil {
		return nil, err
	}

	var af *metal.AddressFamily
	if req.AddressFamily != nil {
		switch *req.AddressFamily {
		case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4:
			af = pointer.Pointer(metal.IPv4AddressFamily)
		case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6:
			af = pointer.Pointer(metal.IPv6AddressFamily)
		case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_UNSPECIFIED:
			return nil, fmt.Errorf("unsupported addressfamily")
		default:
			return nil, fmt.Errorf("unsupported addressfamily")
		}

		if !slices.Contains(nw.Prefixes.AddressFamilies(), *af) {
			return nil, fmt.Errorf("there is no prefix for the given addressfamily:%s present in network:%s %s", *af, req.Network, nw.Prefixes.AddressFamilies())
		}
		if req.Ip != nil {
			return nil, fmt.Errorf("it is not possible to specify specificIP and addressfamily")
		}
	}

	// for private, unshared networks the project id must be the same
	// for external networks the project id is not checked
	if !nw.Shared && nw.ParentNetworkID != "" && p.Meta.Id != nw.ProjectID {
		return nil, fmt.Errorf("can not allocate ip for project %q because network belongs to %q and the network is not shared", p.Meta.Id, nw.ProjectID)
	}

	var (
		ipAddress    string
		ipParentCidr string
	)

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

	ipType := metal.Ephemeral
	if req.Type != nil {
		switch *req.Type {
		case apiv2.IPType_IP_TYPE_EPHEMERAL:
			ipType = metal.Ephemeral
		case apiv2.IPType_IP_TYPE_STATIC:
			ipType = metal.Static
		case apiv2.IPType_IP_TYPE_UNSPECIFIED:
			return nil, fmt.Errorf("given ip type is not supported:%s", req.Type.String())
		}
	}

	r.r.log.Info("allocated ip in ipam", "ip", ipAddress, "network", nw.ID, "type", ipType)

	uuid, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	ip := &metal.IP{
		AllocationUUID:   uuid.String(),
		IPAddress:        ipAddress,
		ParentPrefixCidr: ipParentCidr,
		Name:             name,
		Description:      description,
		NetworkID:        nw.ID,
		ProjectID:        projectID,
		Type:             ipType,
		Tags:             tags,
	}

	r.r.log.Info("create ip in db", "ip", ip)

	resp, err := r.r.ds.IP().Create(ctx, ip)
	if err != nil {
		return nil, err
	}

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
			return nil, fmt.Errorf("ip type cannot be unspecified: %s", rq.Type)
		}
		new.Type = t
	}
	if rq.Labels != nil {
		tags := tag.TagMap(rq.Labels.Labels).Slice()
		new.Tags = tags
	}

	err = r.r.ds.IP().Update(ctx, &new)
	if err != nil {
		return nil, err
	}

	return &new, nil
}

func (r *ipRepository) delete(ctx context.Context, e *metal.IP) error {
	info, err := r.r.async.NewIPDeleteTask(e.AllocationUUID, e.IPAddress, e.ProjectID)
	if err != nil {
		return err
	}

	r.r.log.Info("ip delete queued", "info", info)

	return nil
}

func (r *ipRepository) find(ctx context.Context, rq *apiv2.IPQuery) (*metal.IP, error) {
	ip, err := r.r.ds.IP().Find(ctx, r.scopedFilters(queries.IpFilter(rq))...)
	if err != nil {
		return nil, err
	}

	return ip, nil
}

func (r *ipRepository) list(ctx context.Context, rq *apiv2.IPQuery) ([]*metal.IP, error) {
	ip, err := r.r.ds.IP().List(ctx, r.scopedFilters(queries.IpFilter(rq))...)
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

	af := metal.IPv4AddressFamily
	if parsedIP.Is6() {
		af = metal.IPv6AddressFamily
	}

	for _, prefix := range parent.Prefixes.OfFamily(af) {
		pfx, err := netip.ParsePrefix(prefix.String())
		if err != nil {
			return "", "", fmt.Errorf("unable to parse prefix: %w", err)
		}

		if !pfx.Contains(parsedIP) {
			continue
		}

		resp, err := r.r.ipam.AcquireIP(ctx, connect.NewRequest(&ipamapiv1.AcquireIPRequest{PrefixCidr: prefix.String(), Ip: &specificIP}))
		if err != nil {
			return "", "", err
		}

		return resp.Msg.Ip.Ip, prefix.String(), nil
	}

	return "", "", fmt.Errorf("specific ip %s not contained in any of the defined prefixes", specificIP)
}

func (r *ipRepository) allocateRandomIP(ctx context.Context, parent *metal.Network, af *metal.AddressFamily) (ipAddress, parentPrefixCidr string, err error) {
	addressfamily := metal.IPv4AddressFamily
	if af != nil {
		addressfamily = *af
	} else if len(parent.Prefixes.AddressFamilies()) == 1 {
		addressfamily = parent.Prefixes.AddressFamilies()[0]
	}

	for _, prefix := range parent.Prefixes.OfFamily(addressfamily) {
		resp, err := r.r.ipam.AcquireIP(ctx, connect.NewRequest(&ipamapiv1.AcquireIPRequest{PrefixCidr: prefix.String()}))
		if err != nil {
			if errorutil.IsNotFound(err) {
				continue
			}
			return "", "", err
		}

		return resp.Msg.Ip.Ip, prefix.String(), nil
	}

	return "", "", fmt.Errorf("cannot allocate random free ip in ipam, no ips left in network:%s af:%s parent afs:%#v", parent.ID, addressfamily, parent.Prefixes.AddressFamilies())
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

	ip := &apiv2.IP{
		Ip:          metalIP.IPAddress,
		Uuid:        metalIP.AllocationUUID,
		Name:        metalIP.Name,
		Description: metalIP.Description,
		Network:     metalIP.NetworkID,
		Project:     metalIP.ProjectID,
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

	_, err = r.ipam.ReleaseIP(ctx, connect.NewRequest(&ipamapiv1.ReleaseIPRequest{PrefixCidr: metalIP.ParentPrefixCidr, Ip: metalIP.IPAddress}))
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

func (r *ipRepository) scopedFilters(filter generic.EntityQuery) []generic.EntityQuery {
	var qs []generic.EntityQuery
	r.r.log.Info("scopedFilters", "scope", r.scope)
	if r.scope != nil {
		qs = append(qs, queries.IpProjectScoped(r.scope.projectID))
	}
	if filter != nil {
		qs = append(qs, filter)
	}
	return qs
}
