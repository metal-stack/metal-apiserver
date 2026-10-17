package q

import (
	"fmt"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/tag"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// ipPaths precomputes the JSON paths into an ip so the same strings are reused
// across queries instead of being recomputed with reflection.
var ipPaths = struct {
	IPAddress        string
	AllocationUUID   string
	Namespace        string
	ParentPrefixCidr string
	Name             string
	ProjectID        string
	NetworkID        string
	Type             string
	Tags             string
}{
	IPAddress:        pg.SelectorPath[*metal.IP]("IPAddress"),
	AllocationUUID:   pg.SelectorPath[*metal.IP]("AllocationUUID"),
	Namespace:        pg.SelectorPath[*metal.IP]("Namespace"),
	ParentPrefixCidr: pg.SelectorPath[*metal.IP]("ParentPrefixCidr"),
	Name:             pg.SelectorPath[*metal.IP]("Name"),
	ProjectID:        pg.SelectorPath[*metal.IP]("ProjectID"),
	NetworkID:        pg.SelectorPath[*metal.IP]("NetworkID"),
	Type:             pg.SelectorPath[*metal.IP]("Type"),
	Tags:             pg.SelectorPath[*metal.IP]("Tags"),
}

// IpFilter converts the given ip query into a list of postgres query filters.
// It returns nil when the query is nil.
//
// The filters produced here mirror the RethinkDB IpFilter (see the sibling
// `queries` package). Scalar field equality uses the `=` operator; tag
// membership (machine id and labels) uses the `@>` containment operator; and
// the address-family check matches an ip address that looks like IPv4
// (contains a dot) or IPv6 (contains a colon) with LIKE_PATTERN.
func IpFilter(rq *apiv2.IPQuery) []pg.QueryFilter {
	if rq == nil {
		return nil
	}

	var filters []pg.QueryFilter

	if rq.Project != nil {
		filters = append(filters, pg.QueryFilter{Path: ipPaths.ProjectID, Op: "=", Value: *rq.Project})
	}
	if rq.Ip != nil {
		filters = append(filters, pg.QueryFilter{Path: ipPaths.IPAddress, Op: "=", Value: *rq.Ip})
	}
	if rq.Uuid != nil {
		filters = append(filters, pg.QueryFilter{Path: ipPaths.AllocationUUID, Op: "=", Value: *rq.Uuid})
	}
	if rq.Name != nil {
		filters = append(filters, pg.QueryFilter{Path: ipPaths.Name, Op: "=", Value: *rq.Name})
	}
	if rq.Namespace != nil {
		filters = append(filters, pg.QueryFilter{Path: ipPaths.Namespace, Op: "=", Value: *rq.Namespace})
	}
	if rq.Network != nil {
		filters = append(filters, pg.QueryFilter{Path: ipPaths.NetworkID, Op: "=", Value: *rq.Network})
	}
	if rq.ParentPrefixCidr != nil {
		filters = append(filters, pg.QueryFilter{Path: ipPaths.ParentPrefixCidr, Op: "=", Value: *rq.ParentPrefixCidr})
	}

	if rq.Machine != nil {
		filters = append(filters, pg.QueryFilter{
			Path:  ipPaths.Tags,
			Op:    "@>",
			Value: containsJSON(ipPaths.Tags, []string{fmt.Sprintf("%s=%s", tag.MachineID, *rq.Machine)}),
		})
	}
	if rq.Labels != nil {
		for key, value := range rq.Labels.Labels {
			filters = append(filters, pg.QueryFilter{
				Path:  ipPaths.Tags,
				Op:    "@>",
				Value: containsJSON(ipPaths.Tags, []string{fmt.Sprintf("%s=%s", key, value)}),
			})
		}
	}

	if rq.Type != nil {
		typeString, err := enum.GetStringValue(*rq.Type)
		if err == nil {
			filters = append(filters, pg.QueryFilter{Path: ipPaths.Type, Op: "=", Value: *typeString})
		}
	}

	if rq.AddressFamily != nil {
		switch rq.AddressFamily.String() {
		case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.String():
			filters = append(filters, pg.QueryFilter{Path: ipPaths.IPAddress, Op: "LIKE_PATTERN", Value: "%.%"})
		case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.String():
			filters = append(filters, pg.QueryFilter{Path: ipPaths.IPAddress, Op: "LIKE_PATTERN", Value: "%:%"})
		}
	}

	return filters
}

// IpProjectScoped is the postgres representation of the project scope filter
// and mirrors queries.IpProjectScoped.
func IpProjectScoped(project string) []pg.QueryFilter {
	return []pg.QueryFilter{{Path: ipPaths.ProjectID, Op: "=", Value: project}}
}
