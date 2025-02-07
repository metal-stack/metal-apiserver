package ip

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/metal"
	"github.com/metal-stack/api-server/pkg/repository"
	"github.com/metal-stack/api-server/pkg/validate"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"

	"github.com/metal-stack/metal-lib/pkg/tag"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Repository
}

type ipServiceServer struct {
	log  *slog.Logger
	repo *repository.Repository
}

func New(c Config) apiv2connect.IPServiceHandler {
	return &ipServiceServer{
		log:  c.Log.WithGroup("ipService"),
		repo: c.Repo,
	}
}

func (i *ipServiceServer) Get(ctx context.Context, rq *connect.Request[apiv2.IPServiceGetRequest]) (*connect.Response[apiv2.IPServiceGetResponse], error) {
	i.log.Debug("get", "ip", rq)
	req := rq.Msg

	// Project is already checked in the tenant-interceptor, ipam must not be consulted
	resp, err := i.repo.IP(repository.ProjectScope(req.Project)).Get(ctx, req.Ip)
	if err != nil {
		if generic.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, err
	}

	return connect.NewResponse(&apiv2.IPServiceGetResponse{
		Ip: convert(resp),
	}), nil
}

// List implements v1.IPServiceServer
func (i *ipServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.IPServiceListRequest]) (*connect.Response[apiv2.IPServiceListResponse], error) {
	i.log.Debug("list", "ip", rq)
	req := rq.Msg

	resp, err := i.repo.IP(repository.ProjectScope(req.Project)).List(ctx, req)
	if err != nil {
		return nil, err
	}

	var res []*apiv2.IP
	for _, ip := range resp {

		m := tag.NewTagMap(ip.Tags)
		if _, ok := m.Value(tag.MachineID); ok {
			// we do not want to show machine ips (e.g. firewall public ips)
			continue
		}

		res = append(res, convert(ip))
	}

	return connect.NewResponse(&apiv2.IPServiceListResponse{
		Ips: res,
	}), nil
}

// Delete implements v1.IPServiceServer
func (i *ipServiceServer) Delete(ctx context.Context, rq *connect.Request[apiv2.IPServiceDeleteRequest]) (*connect.Response[apiv2.IPServiceDeleteResponse], error) {
	i.log.Debug("delete", "ip", rq)
	req := rq.Msg

	// TODO also delete in go-ipam in one transaction
	// maybe reuse asyncActor from metal-api
	// Ensure that only this ip with the same uuid gets deleted
	ip, err := i.repo.IP(repository.ProjectScope(req.Project)).Delete(ctx, req.Ip)
	if err != nil {
		if generic.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, err
	}

	return connect.NewResponse(&apiv2.IPServiceDeleteResponse{
		Ip: convert(ip),
	}), nil
}

func (i *ipServiceServer) Create(ctx context.Context, rq *connect.Request[apiv2.IPServiceCreateRequest]) (*connect.Response[apiv2.IPServiceCreateResponse], error) {
	i.log.Debug("create", "ip", rq)
	req := rq.Msg

	if req.Network == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("network should not be empty"))
	}
	if req.Project == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("project should not be empty"))
	}

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
	tags := req.Tags
	if req.MachineId != nil {
		tags = append(tags, tag.New(tag.MachineID, *req.MachineId))
	}
	// Ensure no duplicates
	tags = tag.NewTagMap(tags).Slice()

	p, err := i.repo.Project().Get(ctx, req.Project)
	if err != nil {
		// FIXME map generic errors to connect errors
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	projectID := p.Meta.Id

	nw, err := i.repo.Network(repository.ProjectScope(req.Project)).Get(ctx, req.Network)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var af metal.AddressFamily
	if req.AddressFamily != nil {
		err := validate.ValidateAddressFamily(*req.AddressFamily)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		switch *req.AddressFamily {
		case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4:
			af = metal.IPv4AddressFamily
		case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6:
			af = metal.IPv6AddressFamily
		case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_UNSPECIFIED:
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported addressfamily"))
		}

		if !slices.Contains(nw.AddressFamilies, af) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("there is no prefix for the given addressfamily:%s present in network:%s", string(*req.AddressFamily), req.Network))
		}
		if req.Ip != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("it is not possible to specify specificIP and addressfamily"))
		}
	}

	// for private, unshared networks the project id must be the same
	// for external networks the project id is not checked
	if !nw.Shared && nw.ParentNetworkID != "" && p.Meta.Id != nw.ProjectID {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("can not allocate ip for project %q because network belongs to %q and the network is not shared", p.Meta.Id, nw.ProjectID))
	}

	// TODO: Following operations should span a database transaction if possible

	var (
		ipAddress    string
		ipParentCidr string
	)

	if req.Ip == nil {
		ipAddress, ipParentCidr, err = i.repo.IP(repository.ProjectScope(projectID)).AllocateRandomIP(ctx, nw, &af)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	} else {
		ipAddress, ipParentCidr, err = i.repo.IP(repository.ProjectScope(projectID)).AllocateSpecificIP(ctx, nw, *req.Ip)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
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
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("given ip type is not supported:%s", req.Type.String()))
		}
	}

	i.log.Info("allocated ip in ipam", "ip", ipAddress, "network", nw.ID, "type", ipType)

	ip := &metal.IP{
		IPAddress:        ipAddress,
		ParentPrefixCidr: ipParentCidr,
		Name:             name,
		Description:      description,
		NetworkID:        nw.ID,
		ProjectID:        projectID,
		Type:             ipType,
		Tags:             tags,
	}

	created, err := i.repo.IP(repository.ProjectScope(projectID)).Create(ctx, ip)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&apiv2.IPServiceCreateResponse{Ip: convert(created)}), nil
}

// Static implements v1.IPServiceServer
func (i *ipServiceServer) Update(ctx context.Context, rq *connect.Request[apiv2.IPServiceUpdateRequest]) (*connect.Response[apiv2.IPServiceUpdateResponse], error) {
	i.log.Debug("update", "ip", rq)

	req := rq.Msg

	ip, err := i.repo.IP(repository.ProjectScope(req.Project)).Update(ctx, req)
	if err != nil {
		if generic.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, err
	}

	return connect.NewResponse(&apiv2.IPServiceUpdateResponse{Ip: convert(ip)}), nil
}

func convert(resp *metal.IP) *apiv2.IP {
	t := apiv2.IPType_IP_TYPE_UNSPECIFIED
	switch resp.Type {
	case metal.Ephemeral:
		t = apiv2.IPType_IP_TYPE_EPHEMERAL
	case metal.Static:
		t = apiv2.IPType_IP_TYPE_STATIC
	}

	ip := &apiv2.IP{
		Ip:          resp.IPAddress,
		Uuid:        resp.AllocationUUID,
		Name:        resp.Name,
		Description: resp.Description,
		Network:     resp.NetworkID,
		Project:     resp.ProjectID,
		Type:        t,
		Tags:        resp.Tags,
		CreatedAt:   timestamppb.New(time.Time(resp.Created)),
		UpdatedAt:   timestamppb.New(time.Time(resp.Changed)),
	}
	return ip
}
