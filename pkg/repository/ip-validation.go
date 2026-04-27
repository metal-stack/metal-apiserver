package repository

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/tag"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

func (r *ipRepository) validateCreate(ctx context.Context, req *apiv2.IPServiceCreateRequest) error {
	var errs []error

	errs = validate(errs, req.Project != "", "project should not be empty")
	errs = validate(errs, req.Network != "", "network should not be empty")

	nw, err := r.s.ds.Network().Get(ctx, req.Network)
	if err != nil {
		return err
	}

	if nw.ProjectID != "" && nw.ProjectID != req.Project {
		return fmt.Errorf("not allowed to create ip with project %s in network %s scoped to project %s", req.Project, req.Network, nw.ProjectID)
	}

	// for private, unshared networks the project id must be the same
	// for external and underlay networks the project id is not checked
	if nw.ProjectID != req.Project {
		switch *nw.NetworkType {
		case metal.NetworkTypeChildShared, metal.NetworkTypeExternal, metal.NetworkTypeUnderlay:
			// this is fine
		default:
			return fmt.Errorf("can not allocate ip for project %q because network belongs to %q and the network is of type:%s", req.Project, nw.ProjectID, *nw.NetworkType)
		}
	}

	if req.Ip != nil {
		existingIP, err := r.s.ds.IP().Get(ctx, metal.CreateNamespacedIPAddress(nw.Namespace, *req.Ip))
		if err == nil || existingIP != nil {
			return fmt.Errorf("given ip %q is already allocated", *req.Ip)
		}
		parsedIP, err := netip.ParseAddr(*req.Ip)
		if err != nil {
			return err
		}

		var partofNetworkPrefixes bool
		for _, pfx := range nw.Prefixes {
			parsedPrefix, err := netip.ParsePrefix(pfx.String())
			if err != nil {
				return err
			}

			if parsedPrefix.Contains(parsedIP) {
				partofNetworkPrefixes = true
			}
		}
		if !partofNetworkPrefixes {
			return fmt.Errorf("specific ip %q is not contained in any of the prefixes of network %q", *req.Ip, nw.ID)
		}
	} else {
		if err := r.s.UnscopedNetwork().AdditionalMethods().ipsAvailable(ctx, nw.ID); err != nil {
			return err
		}
	}

	if req.AddressFamily != nil {
		convertedAf, err := metal.ToAddressFamily(*req.AddressFamily)
		if err != nil {
			return err
		}

		if !slices.Contains(nw.Prefixes.AddressFamilies(), convertedAf) {
			return fmt.Errorf("there is no prefix for the given addressfamily:%s present in network:%s %s", convertedAf, req.Network, nw.Prefixes.AddressFamilies())
		}
		if req.Ip != nil {
			return fmt.Errorf("it is not possible to specify specificIP and addressfamily")
		}
	}

	switch nt := *nw.NetworkType; nt {
	case metal.NetworkTypeChild, metal.NetworkTypeChildShared, metal.NetworkTypeExternal:
		// all fine
	case metal.NetworkTypeUnderlay:
		// Only admins
		tok, ok := token.TokenFromContext(ctx)

		if !ok || tok == nil {
			return errorutil.Unauthenticated("no token found in request")
		}
		if tok.AdminRole == nil || *tok.AdminRole != apiv2.AdminRole_ADMIN_ROLE_EDITOR {
			return errorutil.PermissionDenied("only admin editors can allocate ips from an underlay network")
		}
	default:
		return errorutil.PermissionDenied("given networktype %q is not allowed", nt)
	}

	return errors.Join(errs...)
}

func (r *ipRepository) validateUpdate(ctx context.Context, req *apiv2.IPServiceUpdateRequest, ip *metal.IP) error {
	if req.Type != nil {
		if ip.Type == metal.Static && *req.Type != apiv2.IPType_IP_TYPE_STATIC {
			return fmt.Errorf("cannot change type of ip address from static to ephemeral")
		}
	}

	return nil
}

func (r *ipRepository) validateDelete(ctx context.Context, ip *metal.IP) error {
	var errs []error

	errs = validate(errs, ip.IPAddress != "", "ipaddress is empty")
	errs = validate(errs, ip.AllocationUUID != "", "allocationUUID is empty")
	errs = validate(errs, ip.ProjectID != "", "projectId is empty")

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	for _, t := range ip.Tags {
		if strings.HasPrefix(t, tag.MachineID) {
			return fmt.Errorf("ip with machine scope cannot be deleted")
		}
	}

	return nil
}
