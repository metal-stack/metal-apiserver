package repository

import (
	"context"
	"net/netip"
	"slices"

	"connectrpc.com/connect"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/metal"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
)

type networkRepository struct {
	r     *Repository
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

func (r *networkRepository) Delete(ctx context.Context, id string) (*metal.Network, error) {
	nw, err := r.Get(ctx, id)
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

func (r *networkRepository) Create(ctx context.Context, nw *metal.Network) (*metal.Network, error) {
	var afs metal.AddressFamilies
	for _, p := range nw.Prefixes {
		parsed, err := netip.ParsePrefix(p.String())
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
	}
	nw.AddressFamilies = afs

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
