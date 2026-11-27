package q

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// filesystemLayoutPathID is the JSON path to a filesystem layout's id.
var filesystemLayoutPathID = pg.SelectorPath[*metal.FilesystemLayout]("ID")

// FileSystemLayoutFilter converts the given filesystem layout list request into
// a list of postgres query filters. It returns nil when the request is nil.
//
// It mirrors the RethinkDB FileSystemLayoutFilter (see the sibling `queries`
// package).
func FileSystemLayoutFilter(rq *apiv2.FilesystemServiceListRequest) []pg.QueryFilter {
	if rq == nil {
		return nil
	}

	var filters []pg.QueryFilter

	if rq.Id != nil {
		filters = append(filters, pg.QueryFilter{Path: filesystemLayoutPathID, Op: "=", Value: *rq.Id})
	}

	return filters
}
