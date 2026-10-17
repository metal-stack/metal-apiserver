package q

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// sizeReservationPaths precomputes the JSON paths into a size reservation.
var sizeReservationPaths = struct {
	ID           string
	SizeID       string
	Name         string
	Description  string
	Labels       string
	ProjectID    string
	PartitionIDs string
}{
	ID:           pg.SelectorPath[*metal.SizeReservation]("ID"),
	SizeID:       pg.SelectorPath[*metal.SizeReservation]("SizeID"),
	Name:         pg.SelectorPath[*metal.SizeReservation]("Name"),
	Description:  pg.SelectorPath[*metal.SizeReservation]("Description"),
	Labels:       pg.SelectorPath[*metal.SizeReservation]("Labels"),
	ProjectID:    pg.SelectorPath[*metal.SizeReservation]("ProjectID"),
	PartitionIDs: pg.SelectorPath[*metal.SizeReservation]("PartitionIDs"),
}

// SizeReservationFilter converts the given size reservation query into a list
// of postgres query filters. It returns nil when the query is nil.
//
// It mirrors the RethinkDB SizeReservationFilter (see the sibling `queries`
// package). Scalar field equality uses the `=` operator; label membership and
// partition membership use the `@>` containment operator.
func SizeReservationFilter(rq *apiv2.SizeReservationQuery) []pg.QueryFilter {
	if rq == nil {
		return nil
	}

	var filters []pg.QueryFilter

	if rq.Id != nil {
		filters = append(filters, pg.QueryFilter{Path: sizeReservationPaths.ID, Op: "=", Value: *rq.Id})
	}
	if rq.Size != nil {
		filters = append(filters, pg.QueryFilter{Path: sizeReservationPaths.SizeID, Op: "=", Value: *rq.Size})
	}
	if rq.Name != nil {
		filters = append(filters, pg.QueryFilter{Path: sizeReservationPaths.Name, Op: "=", Value: *rq.Name})
	}
	if rq.Description != nil {
		filters = append(filters, pg.QueryFilter{Path: sizeReservationPaths.Description, Op: "=", Value: *rq.Description})
	}
	if rq.Labels != nil {
		for key, value := range rq.Labels.Labels {
			filters = append(filters, pg.QueryFilter{Path: sizeReservationPaths.Labels, Op: "@>", Value: containsJSON(sizeReservationPaths.Labels, map[string]string{key: value})})
		}
	}
	if rq.Project != nil {
		filters = append(filters, pg.QueryFilter{Path: sizeReservationPaths.ProjectID, Op: "=", Value: *rq.Project})
	}
	if rq.Partition != nil {
		filters = append(filters, pg.QueryFilter{Path: sizeReservationPaths.PartitionIDs, Op: "@>", Value: containsJSON(sizeReservationPaths.PartitionIDs, []string{*rq.Partition})})
	}

	return filters
}
