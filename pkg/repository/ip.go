package repository

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/metal-stack/api-server/pkg/db/metal"
	"github.com/metal-stack/api-server/pkg/db/queries"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

type ipRepository struct {
	r     *Repository
	scope ProjectScope
}

func (r *ipRepository) Get(ctx context.Context, id string) (*metal.IP, error) {
	ip, err := r.r.ds.IP().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if r.scope != ProjectScope(ip.ProjectID) {
		return nil, fmt.Errorf("TODO: NOT YOUR ENTITY NOT FOUND")
	}

	return ip, nil
}

func (r *ipRepository) Update(ctx context.Context, rq *apiv2.IPServiceUpdateRequest) (*metal.IP, error) {
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
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("ip type cannot be unspecified: %s", rq.Type))
		}
		new.Type = t
	}
	new.Tags = rq.Tags

	err = r.r.ds.IP().Update(ctx, &new, old)
	if err != nil {
		return nil, err
	}

	return &new, nil
}

func (r *ipRepository) Delete(ctx context.Context, id string) (*metal.IP, error) {
	ip, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	err = r.r.ds.IP().Delete(ctx, ip)
	if err != nil {
		return nil, err
	}

	return ip, nil
}

func (r *ipRepository) List(ctx context.Context, rq *apiv2.IPServiceListRequest) ([]*metal.IP, error) {
	ip, err := r.r.ds.IP().Search(ctx, queries.IpFilter(rq))
	if err != nil {
		return nil, err
	}

	return ip, nil
}
