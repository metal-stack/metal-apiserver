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
	asyncclient "github.com/metal-stack/api-server/pkg/async/client"
	"github.com/metal-stack/api-server/pkg/db/metal"
	"github.com/metal-stack/api-server/pkg/db/queries"
	"github.com/metal-stack/api-server/pkg/db/validate"
	"github.com/metal-stack/api-server/pkg/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamapiv1 "github.com/metal-stack/go-ipam/api/v1"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/metal-stack/metal-lib/pkg/tag"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ipRepository struct {
	r     *Store
	scope *ProjectScope
}

func (r *ipRepository) Get(ctx context.Context, id string) (*metal.IP, error) {
	ip, err := r.r.ds.IP().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	err = r.MatchScope(ip)
	if err != nil {
		return nil, err
	}

	return ip, nil
}

func (r *ipRepository) MatchScope(ip *metal.IP) error {
	if r.scope == nil {
		return nil
	}
	if r.scope.projectID == ip.ProjectID {
		return nil
	}
	return errorutil.NotFound("ip:%s for project:%s not found", ip.IPAddress, ip.ProjectID)
}

func (r *ipRepository) ValidateCreate(ctx context.Context, req *apiv2.IPServiceCreateRequest) (*Validated[*apiv2.IPServiceCreateRequest], error) {
	if req.Network == "" {
		return nil, errorutil.InvalidArgument("network should not be empty")
	}
	if req.Project == "" {
		return nil, errorutil.InvalidArgument("project should not be empty")
	}

	return &Validated[*apiv2.IPServiceCreateRequest]{
		message: req,
	}, nil
}

func (r *ipRepository) ValidateUpdate(ctx context.Context, req *apiv2.IPServiceUpdateRequest) (*Validated[*apiv2.IPServiceUpdateRequest], error) {
	return &Validated[*apiv2.IPServiceUpdateRequest]{
		message: req,
	}, nil
}

func (r *ipRepository) Create(ctx context.Context, rq *Validated[*apiv2.IPServiceCreateRequest]) (*metal.IP, error) {
	r.r.log.Debug("")
	var (
		name        string
		description string
	)
	req := rq.message

	if req.Name != nil {
		name = *req.Name
	}
	if req.Description != nil {
		description = *req.Description
	}
	tags := labelsToTags(req.Labels)
	if req.MachineId != nil {
		tags = append(tags, tag.New(tag.MachineID, *req.MachineId))
	}
	// Ensure no duplicates
	tags = tag.NewTagMap(tags).Slice()

	p, err := r.r.Project(&req.Project).Get(ctx, req.Project)
	if err != nil {
		// FIXME map generic errors to connect errors
		return nil, err
	}
	projectID := p.Meta.Id

	nw, err := r.r.Network(&req.Project).Get(ctx, req.Network)
	if err != nil {
		return nil, err
	}

	var af *metal.AddressFamily
	if req.AddressFamily != nil {
		err := validate.ValidateAddressFamily(*req.AddressFamily)
		if err != nil {
			return nil, errorutil.NewInvalidArgument(err)
		}
		switch *req.AddressFamily {
		case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4:
			af = pointer.Pointer(metal.IPv4AddressFamily)
		case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6:
			af = pointer.Pointer(metal.IPv6AddressFamily)
		case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_UNSPECIFIED:
			return nil, errorutil.InvalidArgument("unsupported addressfamily")
		}

		if !slices.Contains(nw.Prefixes.AddressFamilies(), *af) {
			return nil, errorutil.InvalidArgument("there is no prefix for the given addressfamily:%s present in network:%s %s", *af, req.Network, nw.Prefixes.AddressFamilies())
		}
		if req.Ip != nil {
			return nil, errorutil.InvalidArgument("it is not possible to specify specificIP and addressfamily")
		}
	}

	// for private, unshared networks the project id must be the same
	// for external networks the project id is not checked
	if !nw.Shared && nw.ParentNetworkID != "" && p.Meta.Id != nw.ProjectID {
		return nil, errorutil.InvalidArgument("can not allocate ip for project %q because network belongs to %q and the network is not shared", p.Meta.Id, nw.ProjectID)
	}

	var (
		ipAddress    string
		ipParentCidr string
	)

	if req.Ip == nil {
		ipAddress, ipParentCidr, err = r.AllocateRandomIP(ctx, nw, af)
		if err != nil {
			return nil, err
		}
	} else {
		ipAddress, ipParentCidr, err = r.AllocateSpecificIP(ctx, nw, *req.Ip)
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
			return nil, errorutil.InvalidArgument("given ip type is not supported:%s", req.Type.String())
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

	resp, err := r.r.ds.IP().Create(ctx, ip)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (r *ipRepository) Update(ctx context.Context, req *Validated[*apiv2.IPServiceUpdateRequest]) (*metal.IP, error) {
	rq := req.message
	old, err := r.Get(ctx, rq.Ip)
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
		tags := labelsToTags(rq.Labels)
		new.Tags = tags
	}

	err = r.r.ds.IP().Update(ctx, &new, old)
	if err != nil {
		return nil, err
	}

	return &new, nil
}

func (r *ipRepository) Delete(ctx context.Context, ip *metal.IP) (*metal.IP, error) {
	ip, err := r.Get(ctx, ip.GetID())
	if err != nil {
		return nil, err
	}

	info, err := r.r.async.NewIPDeleteTask(ip.AllocationUUID, ip.IPAddress, ip.ProjectID)
	if err != nil {
		return nil, err
	}
	r.r.log.Info("ip delete queued", "info", info)

	return ip, nil
}

func (r *ipRepository) Find(ctx context.Context, rq *apiv2.IPQuery) (*metal.IP, error) {
	ip, err := r.r.ds.IP().Find(ctx, queries.IpFilter(rq))
	if err != nil {
		return nil, err
	}

	return ip, nil
}

func (r *ipRepository) List(ctx context.Context, rq *apiv2.IPQuery) ([]*metal.IP, error) {
	ip, err := r.r.ds.IP().List(ctx, queries.IpFilter(rq))
	if err != nil {
		return nil, err
	}

	return ip, nil
}

func (r *ipRepository) AllocateSpecificIP(ctx context.Context, parent *metal.Network, specificIP string) (ipAddress, parentPrefixCidr string, err error) {
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

	return "", "", errorutil.InvalidArgument("specific ip %s not contained in any of the defined prefixes", specificIP)
}

func (r *ipRepository) AllocateRandomIP(ctx context.Context, parent *metal.Network, af *metal.AddressFamily) (ipAddress, parentPrefixCidr string, err error) {
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

	return "", "", errorutil.InvalidArgument("cannot allocate random free ip in ipam, no ips left in network:%s af:%s parent afs:%#v", parent.ID, addressfamily, parent.Prefixes.AddressFamilies())
}
func (r *ipRepository) ConvertToInternal(ip *apiv2.IP) (*metal.IP, error) {

	panic("unimplemented")
}
func (r *ipRepository) ConvertToProto(metalIP *metal.IP) (*apiv2.IP, error) {
	t := apiv2.IPType_IP_TYPE_UNSPECIFIED
	switch metalIP.Type {
	case metal.Ephemeral:
		t = apiv2.IPType_IP_TYPE_EPHEMERAL
	case metal.Static:
		t = apiv2.IPType_IP_TYPE_STATIC
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
			Labels:    tagsToLabels(metalIP.Tags),
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

type IPProcessor struct {
	R *Store
}

func (ip *IPProcessor) ProcessTask(ctx context.Context, t *asynq.Task) error {

	// use p.r.IPDeleteAction here
	return nil
}

func (r *Store) IpDeleteAction(ctx context.Context, t *asynq.Task) error {

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

func labelsToTags(labels *apiv2.Labels) []string {
	if labels == nil {
		return nil
	}
	var tags []string
	for key, value := range labels.Labels {
		tags = append(tags, fmt.Sprintf("%s=%s", key, value))
	}
	return tags
}
func tagsToLabels(tags []string) *apiv2.Labels {
	if len(tags) == 0 {
		return nil
	}
	var labels = &apiv2.Labels{
		Labels: map[string]string{},
	}
	for _, t := range tags {
		key, value, _ := strings.Cut(t, "=")
		labels.Labels[key] = value
	}
	return labels
}
