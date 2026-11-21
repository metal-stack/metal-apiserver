package q

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// partitionPathID is the JSON path to a partition's id.
var partitionPathID = pg.SelectorPath[*metal.Partition]("ID")

// PartitionFilter converts the given partition query into a list of postgres
// query filters. It returns nil when the query is nil.
//
// It mirrors the RethinkDB PartitionFilter (see the sibling `queries` package).
func PartitionFilter(rq *apiv2.PartitionQuery) []pg.QueryFilter {
	if rq == nil {
		return nil
	}

	var filters []pg.QueryFilter

	if rq.Id != nil {
		filters = append(filters, pg.QueryFilter{Path: partitionPathID, Op: "=", Value: *rq.Id})
	}

	return filters
}
