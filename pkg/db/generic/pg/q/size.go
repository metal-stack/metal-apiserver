package q

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// sizePaths precomputes the JSON paths into a size.
var sizePaths = struct {
	ID          string
	Name        string
	Description string
	Labels      string
}{
	ID:          pg.SelectorPath[*metal.Size]("ID"),
	Name:        pg.SelectorPath[*metal.Size]("Name"),
	Description: pg.SelectorPath[*metal.Size]("Description"),
	Labels:      pg.SelectorPath[*metal.Size]("Labels"),
}

// SizeFilter converts the given size query into a list of postgres query
// filters. It returns nil when the query is nil.
//
// It mirrors the RethinkDB SizeFilter (see the sibling `queries` package).
// Scalar field equality uses the `=` operator; label membership uses the `@>`
// containment operator.
func SizeFilter(rq *apiv2.SizeQuery) []pg.QueryFilter {
	if rq == nil {
		return nil
	}

	var filters []pg.QueryFilter

	if rq.Id != nil {
		filters = append(filters, pg.QueryFilter{Path: sizePaths.ID, Op: "=", Value: *rq.Id})
	}
	if rq.Description != nil {
		filters = append(filters, pg.QueryFilter{Path: sizePaths.Description, Op: "=", Value: *rq.Description})
	}
	if rq.Name != nil {
		filters = append(filters, pg.QueryFilter{Path: sizePaths.Name, Op: "=", Value: *rq.Name})
	}
	if rq.Labels != nil {
		for key, value := range rq.Labels.Labels {
			filters = append(filters, pg.QueryFilter{Path: sizePaths.Labels, Op: "@>", Value: containsJSON(sizePaths.Labels, map[string]string{key: value})})
		}
	}

	return filters
}
