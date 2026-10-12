// Package filters builds backend-agnostic query filters (routing.Filter) for
// metal entities from their API query messages, covering both the RethinkDB and
// the Postgres backend.
//
// Centralizing this here keeps the repository implementations free of
// backend-specific query plumbing: a repository only describes which entity and
// API query it wants, and routing.Find/routing.List takes care of the rest.
package filters

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	pgq "github.com/metal-stack/metal-apiserver/pkg/db/generic/pg/q"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/db/routing"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Machine returns the machine query for both backends. A non-empty scopeProject
// restricts the result to machines allocated to that project.
func Machine(rq *apiv2.MachineQuery, scopeProject string) []routing.Filter {
	var filters []routing.Filter

	if scopeProject != "" {
		filters = append(filters, routing.Filter{
			Rethink:  queries.MachineProjectScoped(scopeProject),
			Postgres: pgq.MachineProjectScoped(scopeProject),
		})
	}

	if f := machineQuery(rq); f != nil {
		filters = append(filters, *f)
	}

	return filters
}

func machineQuery(rq *apiv2.MachineQuery) *routing.Filter {
	if isEmptyQuery(rq) {
		return nil
	}
	rethinkFilter := queries.MachineFilter(rq)
	if rethinkFilter == nil {
		return nil
	}
	return &routing.Filter{
		Rethink:  rethinkFilter,
		Postgres: pgq.MachineFilter(rq),
	}
}

// Network returns the network query for both backends. A non-empty scopeProject
// restricts the result to networks of that project.
func Network(rq *apiv2.NetworkQuery, scopeProject string) []routing.Filter {
	var filters []routing.Filter

	if scopeProject != "" {
		filters = append(filters, routing.Filter{
			Rethink:  queries.NetworkProjectScoped(scopeProject),
			Postgres: pgq.NetworkProjectScoped(scopeProject),
		})
	}

	filters = append(filters, NetworkQuery(rq)...)

	return filters
}

// NetworkQuery returns only the network query filter, without a project scope.
func NetworkQuery(rq *apiv2.NetworkQuery) []routing.Filter {
	if isEmptyQuery(rq) {
		return nil
	}
	rethinkFilter := queries.NetworkFilter(rq)
	if rethinkFilter == nil {
		return nil
	}
	return []routing.Filter{{
		Rethink:  rethinkFilter,
		Postgres: pgq.NetworkFilter(rq),
	}}
}

// isEmptyQuery reports whether a proto query message has no fields set, i.e. it
// expresses no filter at all. The rethinkdb query builders return a non-nil
// closure even for an empty query (which applies no filter), so this avoids
// building a filter with an empty backend representation.
func isEmptyQuery(msg proto.Message) bool {
	if msg == nil {
		return true
	}
	m := msg.ProtoReflect()
	if !m.IsValid() {
		return true
	}

	empty := true
	m.Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool {
		empty = false
		return false
	})

	return empty
}
