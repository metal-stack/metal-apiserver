package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

const entityAlreadyModifiedErrorMessage = "the entity was already modified, please retry"

// Config holds the postgres connection configuration.
type Config struct {
	Pool *pgxpool.Pool
}

// EntityQuery is a postgres-specific query function.
// For the kv-store approach, queries are optional filter expressions
// applied after deserialization. When nil, all records match.
type EntityQuery func(data map[string]any) bool
