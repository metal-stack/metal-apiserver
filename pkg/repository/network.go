package repository

import (
	"context"
	"net/netip"
	"slices"
	"strconv"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/metal"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
)

type networkRepository struct {
	r     *Repostore
	scope ProjectScope
}

func (r *networkRepository) Get(ctx context.Context, id string) (*metal.Network, error) {
	nw, err := r.r.ds.Network().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if nw.Shared || nw.PrivateSuper || nw.ProjectID == "" {
		return nw, nil
	}

	if r.scope != ProjectScope(nw.ProjectID) {
		return nil, generic.NotFound("network with id:%s not found", id)
	}

	return nw, nil
}

func (r *networkRepository) Delete(ctx context.Context, n *metal.Network) (*metal.Network, error) {
	nw, err := r.Get(ctx, n.ID)
	if err != nil {
		return nil, err
	}

	// FIXME delete in ipam with the help of Tx

	err = r.r.ds.Network().Delete(ctx, nw)
	if err != nil {
		return nil, err
	}

	return nw, nil
}

func (r *networkRepository) Create(ctx context.Context, req *apiv2.NetworkServiceCreateRequest) (*metal.Network, error) {

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

func (r *networkRepository) Update(ctx context.Context, msg *apiv2.NetworkServiceUpdateRequest) (*metal.Network, error) {
	panic("unimplemented")
}
func (r *networkRepository) Find(ctx context.Context, query *apiv2.NetworkServiceListRequest) (*metal.Network, error) {
	panic("unimplemented")
}
func (r *networkRepository) List(ctx context.Context, query *apiv2.NetworkServiceListRequest) ([]*metal.Network, error) {
	panic("unimplemented")
}
func (r *networkRepository) ConvertToInternal(msg *apiv2.Network) (*metal.Network, error) {
	panic("unimplemented")
}
func (r *networkRepository) ConvertToProto(e *metal.Network) (*apiv2.Network, error) {
	panic("unimplemented")
}
