package repository

import (
	"context"
	"net/netip"
	"slices"
	"strconv"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

type networkRepository struct {
	r     *Store
	scope *ProjectScope
}

func (r *networkRepository) validateCreate(ctx context.Context, req *apiv2.NetworkServiceCreateRequest) (*validated[*apiv2.NetworkServiceCreateRequest], error) {
	return &validated[*apiv2.NetworkServiceCreateRequest]{
		entity: req,
	}, nil
}

func (r *networkRepository) validateUpdate(ctx context.Context, req *apiv2.NetworkServiceUpdateRequest, old *metal.Network) (*validatedUpdate[*metal.Network, *apiv2.NetworkServiceUpdateRequest], error) {
	return &validatedUpdate[*metal.Network, *apiv2.NetworkServiceUpdateRequest]{
		message: req,
	}, nil
}

func (r *networkRepository) validateDelete(ctx context.Context, e *metal.Network) (*validatedDelete[*metal.Network], error) {
	return &validatedDelete[*metal.Network]{
		entity: e,
	}, nil
}

func (r *networkRepository) get(ctx context.Context, id string) (*metal.Network, error) {
	nw, err := r.r.ds.Network().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if nw.Shared || nw.PrivateSuper || nw.ProjectID == "" {
		return nw, nil
	}

	err = r.matchScope(nw)
	if err != nil {
		return nil, err
	}

	return nw, nil
}
func (r *networkRepository) matchScope(nw *metal.Network) error {
	if r.scope == nil {
		return nil
	}
	if r.scope.projectID == nw.ProjectID {
		return nil
	}
	return errorutil.NotFound("nw:%s for project:%s not found", nw.ID, nw.ProjectID)
}

func (r *networkRepository) delete(ctx context.Context, n *validatedDelete[*metal.Network]) (*metal.Network, error) {
	// FIXME delete in ipam with the help of Tx

	err := r.r.ds.Network().Delete(ctx, n.entity)
	if err != nil {
		return nil, err
	}

	return n.entity, nil
}

func (r *networkRepository) create(ctx context.Context, rq *validated[*apiv2.NetworkServiceCreateRequest]) (*metal.Network, error) {

	req := rq.entity
	var (
		id       string
		afs      metal.AddressFamilies
		prefixes metal.Prefixes
	)
	for _, p := range req.Prefixes {
		parsed, err := netip.ParsePrefix(p)
		if err != nil {
			return nil, err
		}
		if parsed.Addr().Is4() {
			if !slices.Contains(afs, metal.IPv4AddressFamily) {
				afs = append(afs, metal.IPv4AddressFamily)
			}
		}
		if parsed.Addr().Is6() {
			if !slices.Contains(afs, metal.IPv6AddressFamily) {
				afs = append(afs, metal.IPv6AddressFamily)
			}
		}
		prefixes = append(prefixes, metal.Prefix{
			IP:     parsed.Addr().String(),
			Length: strconv.Itoa(parsed.Bits()),
		})
	}

	if req.Id == nil {
		uuid, err := uuid.NewV7()
		if err != nil {
			return nil, err
		}
		id = uuid.String()
	} else {
		id = *req.Id
	}

	nw := &metal.Network{
		Base: metal.Base{
			ID: id,
		},
		// FIXME more fields
		Prefixes: prefixes,
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

func (r *networkRepository) update(ctx context.Context, msg *validatedUpdate[*metal.Network, *apiv2.NetworkServiceUpdateRequest]) (*metal.Network, error) {
	panic("unimplemented")
}
func (r *networkRepository) find(ctx context.Context, query *apiv2.NetworkServiceListRequest) (*metal.Network, error) {
	panic("unimplemented")
}
func (r *networkRepository) list(ctx context.Context, query *apiv2.NetworkServiceListRequest) ([]*metal.Network, error) {
	panic("unimplemented")
}
func (r *networkRepository) convertToInternal(msg *apiv2.Network) (*metal.Network, error) {
	panic("unimplemented")
}
func (r *networkRepository) convertToProto(e *metal.Network) (*apiv2.Network, error) {
	panic("unimplemented")
}
