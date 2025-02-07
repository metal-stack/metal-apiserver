package repository

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"connectrpc.com/connect"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/metal"
	"github.com/metal-stack/api-server/pkg/db/queries"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamapiv1 "github.com/metal-stack/go-ipam/api/v1"
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
		return nil, generic.NotFound("ip with id:%s not found", id)
	}

	return ip, nil
}

func (r *ipRepository) Create(ctx context.Context, ip *metal.IP) (*metal.IP, error) {
	ip, err := r.r.ds.IP().Create(ctx, ip)
	if err != nil {
		return nil, err
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

	// FIXME delete in ipam with the help of Tx

	_, err = r.r.ipam.ReleaseIP(ctx, connect.NewRequest(&ipamapiv1.ReleaseIPRequest{Ip: ip.IPAddress, PrefixCidr: ip.ParentPrefixCidr}))
	if err != nil {
		var connectErr *connect.Error
		if errors.As(err, &connectErr) {
			if connectErr.Code() != connect.CodeNotFound {
				return nil, err
			}
		}
	}

	err = r.r.ds.IP().Delete(ctx, ip)
	if err != nil {
		return nil, err
	}

	return ip, nil
}

func (r *ipRepository) List(ctx context.Context, rq *apiv2.IPServiceListRequest) ([]*metal.IP, error) {
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
		var connectErr *connect.Error
		if errors.As(err, &connectErr) {
			if connectErr.Code() == connect.CodeAlreadyExists {
				return "", "", generic.Conflict("ip already allocated")
			}
		}
		if err != nil {
			return "", "", err
		}

		return resp.Msg.Ip.Ip, prefix.String(), nil
	}

	return "", "", fmt.Errorf("specific ip not contained in any of the defined prefixes")
}

func (r *ipRepository) AllocateRandomIP(ctx context.Context, parent *metal.Network, af *metal.AddressFamily) (ipAddress, parentPrefixCidr string, err error) {
	var addressfamily = metal.IPv4AddressFamily
	if af != nil {
		addressfamily = *af
	} else if len(parent.AddressFamilies) == 1 {
		addressfamily = parent.AddressFamilies[0]
	}

	for _, prefix := range parent.Prefixes.OfFamily(addressfamily) {
		resp, err := r.r.ipam.AcquireIP(ctx, connect.NewRequest(&ipamapiv1.AcquireIPRequest{PrefixCidr: prefix.String()}))
		if err != nil {
			var connectErr *connect.Error
			if errors.As(err, &connectErr) {
				if connectErr.Code() == connect.CodeNotFound {
					continue
				}
			}
			return "", "", err
		}

		return resp.Msg.Ip.Ip, prefix.String(), nil
	}

	return "", "", fmt.Errorf("cannot allocate free ip in ipam, no ips left")
}
