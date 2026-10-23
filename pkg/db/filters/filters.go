// Package filters builds backend-agnostic query filters (storage.Filter) for
// metal entities from their API query messages, covering both the RethinkDB and
// the Postgres backend.
//
// Centralizing this here keeps the repository implementations free of
// backend-specific query plumbing: a repository only describes which entity and
// API query it wants, and the entity storage takes care of the rest.
package filters

import (
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	pgq "github.com/metal-stack/metal-apiserver/pkg/db/generic/pg/q"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
	"github.com/metal-stack/metal-apiserver/pkg/db/storage"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Machine returns the machine query for both backends. A non-empty scopeProject
// restricts the result to machines allocated to that project.
func Machine(rq *apiv2.MachineQuery, scopeProject string) []storage.Filter {
	var filters []storage.Filter

	if scopeProject != "" {
		filters = append(filters, storage.Filter{
			Rethink:  generic.EntityQuery(queries.MachineProjectScoped(scopeProject)),
			Postgres: pgq.MachineProjectScoped(scopeProject),
		})
	}

	filters = append(filters, MachineQuery(rq)...)

	return filters
}

// MachineQuery returns only the machine query filter, without a project scope.
func MachineQuery(rq *apiv2.MachineQuery) []storage.Filter {
	return both(isEmptyQuery(rq), queries.MachineFilter(rq), pgq.MachineFilter(rq))
}

// Network returns the network query for both backends. A non-empty scopeProject
// restricts the result to networks of that project.
func Network(rq *apiv2.NetworkQuery, scopeProject string) []storage.Filter {
	var filters []storage.Filter

	if scopeProject != "" {
		filters = append(filters, storage.Filter{
			Rethink:  generic.EntityQuery(queries.NetworkProjectScoped(scopeProject)),
			Postgres: pgq.NetworkProjectScoped(scopeProject),
		})
	}

	filters = append(filters, NetworkQuery(rq)...)

	return filters
}

// NetworkQuery returns only the network query filter, without a project scope.
func NetworkQuery(rq *apiv2.NetworkQuery) []storage.Filter {
	return both(isEmptyQuery(rq), queries.NetworkFilter(rq), pgq.NetworkFilter(rq))
}

// IPByProject returns a filter restricting IPs to those of the given project.
func IPByProject(project string) []storage.Filter {
	return []storage.Filter{{
		Rethink:  generic.EntityQuery(queries.IpProjectScoped(project)),
		Postgres: pgq.IpProjectScoped(project),
	}}
}

// IP returns the IP query filter.
func IP(rq *apiv2.IPQuery) []storage.Filter {
	return both(isEmptyQuery(rq), queries.IpFilter(rq), pgq.IpFilter(rq))
}

// Size returns the size query filter.
func Size(rq *apiv2.SizeQuery) []storage.Filter {
	return both(isEmptyQuery(rq), queries.SizeFilter(rq), pgq.SizeFilter(rq))
}

// SizeReservation returns the size reservation query filter.
func SizeReservation(rq *apiv2.SizeReservationQuery) []storage.Filter {
	return both(isEmptyQuery(rq), queries.SizeReservationFilter(rq), pgq.SizeReservationFilter(rq))
}

// SizeImageConstraint returns the size image constraint query filter.
func SizeImageConstraint(rq *apiv2.SizeImageConstraintQuery) []storage.Filter {
	return both(isEmptyQuery(rq), queries.SizeImageConstraintFilter(rq), pgq.SizeImageConstraintFilter(rq))
}

// Image returns the image query filter.
func Image(rq *apiv2.ImageQuery) []storage.Filter {
	return both(isEmptyQuery(rq), queries.ImageFilter(rq), pgq.ImageFilter(rq))
}

// Partition returns the partition query filter.
func Partition(rq *apiv2.PartitionQuery) []storage.Filter {
	return both(isEmptyQuery(rq), queries.PartitionFilter(rq), pgq.PartitionFilter(rq))
}

// Switch returns the switch query filter.
func Switch(rq *apiv2.SwitchQuery) []storage.Filter {
	return both(isEmptyQuery(rq), queries.SwitchFilter(rq), pgq.SwitchFilter(rq))
}

// FilesystemLayout returns the filesystem layout query filter.
func FilesystemLayout(rq *apiv2.FilesystemServiceListRequest) []storage.Filter {
	return both(isEmptyQuery(rq), queries.FileSystemLayoutFilter(rq), pgq.FileSystemLayoutFilter(rq))
}

// Event returns the provisioning event query filter for a machine.
func Event(machineID string) []storage.Filter {
	return both(machineID == "", queries.EventFilter(machineID), pgq.EventFilter(machineID))
}

// both wraps the RethinkDB and Postgres representations of a query into one
// backend-agnostic filter. An empty query, or a nil RethinkDB builder, means the
// query expresses no filter and yields no filter at all.
func both(empty bool, rethink generic.EntityQuery, postgres []pg.QueryFilter) []storage.Filter {
	if empty || rethink == nil {
		return nil
	}
	return []storage.Filter{{Rethink: rethink, Postgres: postgres}}
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
