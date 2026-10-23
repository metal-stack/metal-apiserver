package q

import (
	apienum "github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// imagePaths precomputes the JSON paths into an image.
var imagePaths = struct {
	ID             string
	OS             string
	Version        string
	Name           string
	Description    string
	Features       string
	Classification string
	Labels         string
}{
	ID:             pg.SelectorPath[*metal.Image]("ID"),
	OS:             pg.SelectorPath[*metal.Image]("OS"),
	Version:        pg.SelectorPath[*metal.Image]("Version"),
	Name:           pg.SelectorPath[*metal.Image]("Name"),
	Description:    pg.SelectorPath[*metal.Image]("Description"),
	Features:       pg.SelectorPath[*metal.Image]("Features"),
	Classification: pg.SelectorPath[*metal.Image]("Classification"),
	Labels:         pg.SelectorPath[*metal.Image]("Labels"),
}

// ImageFilter converts the given image query into a list of postgres query
// filters. It returns nil when the query is nil.
//
// It mirrors the RethinkDB ImageFilter (see the sibling `queries` package).
// Scalar field equality uses the `=` operator; the feature check requires the
// `features` map to contain the requested feature set to true; and label
// membership uses the `@>` containment operator.
func ImageFilter(rq *apiv2.ImageQuery) []pg.QueryFilter {
	if rq == nil {
		return nil
	}

	var filters []pg.QueryFilter

	if rq.Id != nil {
		filters = append(filters, pg.QueryFilter{Path: imagePaths.ID, Op: "=", Value: *rq.Id})
	}
	if rq.Os != nil {
		filters = append(filters, pg.QueryFilter{Path: imagePaths.OS, Op: "=", Value: *rq.Os})
	}
	if rq.Version != nil {
		filters = append(filters, pg.QueryFilter{Path: imagePaths.Version, Op: "=", Value: *rq.Version})
	}
	if rq.Name != nil {
		filters = append(filters, pg.QueryFilter{Path: imagePaths.Name, Op: "=", Value: *rq.Name})
	}
	if rq.Description != nil {
		filters = append(filters, pg.QueryFilter{Path: imagePaths.Description, Op: "=", Value: *rq.Description})
	}

	if rq.Feature != nil {
		feature, err := apienum.GetStringValue(*rq.Feature)
		if err == nil {
			filters = append(filters, pg.QueryFilter{Path: imagePaths.Features + "." + *feature, Op: "=", Value: true})
		}
	}
	if rq.Classification != nil {
		classification, err := apienum.GetStringValue(*rq.Classification)
		if err == nil {
			filters = append(filters, pg.QueryFilter{Path: imagePaths.Classification, Op: "=", Value: *classification})
		}
	}

	if rq.Labels != nil {
		for key, value := range rq.Labels.Labels {
			filters = append(filters, pg.QueryFilter{Path: imagePaths.Labels, Op: "@>", Value: containsJSON(imagePaths.Labels, map[string]string{key: value})})
		}
	}

	return filters
}
