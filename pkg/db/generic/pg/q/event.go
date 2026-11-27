package q

import (
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
)

// EventFilter converts a machine id into a list of postgres query filters. An
// empty machine id matches no event container.
//
// It mirrors the RethinkDB EventFilter (see the sibling `queries` package): the
// event container is keyed by the machine id.
func EventFilter(machineID string) []pg.QueryFilter {
	return []pg.QueryFilter{{Path: pg.PrimaryKeyPath, Op: "=", Value: machineID}}
}
