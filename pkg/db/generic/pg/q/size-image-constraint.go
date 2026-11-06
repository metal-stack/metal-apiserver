package q

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// sizeImageConstraintPaths precomputes the JSON paths into a size image
// constraint.
var sizeImageConstraintPaths = struct {
	ID          string
	Name        string
	Description string
}{
	ID:          pg.SelectorPath[*metal.SizeImageConstraint]("ID"),
	Name:        pg.SelectorPath[*metal.SizeImageConstraint]("Name"),
	Description: pg.SelectorPath[*metal.SizeImageConstraint]("Description"),
}

// SizeImageConstraintFilter converts the given size image constraint query into
// a list of postgres query filters. It returns nil when the query is nil.
//
// It mirrors the RethinkDB SizeImageConstraintFilter (see the sibling
// `queries` package). Note that the size id is both the primary key and the id
// the query matches on.
func SizeImageConstraintFilter(rq *apiv2.SizeImageConstraintQuery) []pg.QueryFilter {
	if rq == nil {
		return nil
	}

	var filters []pg.QueryFilter

	if rq.Size != nil {
		filters = append(filters, pg.QueryFilter{Path: sizeImageConstraintPaths.ID, Op: "=", Value: *rq.Size})
	}
	if rq.Name != nil {
		filters = append(filters, pg.QueryFilter{Path: sizeImageConstraintPaths.Name, Op: "=", Value: *rq.Name})
	}
	if rq.Description != nil {
		filters = append(filters, pg.QueryFilter{Path: sizeImageConstraintPaths.Description, Op: "=", Value: *rq.Description})
	}

	return filters
}
