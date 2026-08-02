package rethinkdb

import (
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

const entityAlreadyModifiedErrorMessage = "the entity was already modified, please retry"

// EntityQuery is a rethinkdb-specific query function that modifies a ReQL term.
type EntityQuery func(q r.Term) r.Term
