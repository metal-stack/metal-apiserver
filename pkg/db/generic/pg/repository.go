package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"uuid"
)

var (
	ErrOptimisticLockConflict = errors.New("optimistic lock conflict: entity has been modified")

	ErrNotFound = errors.New("entity not found")

	// allowedQueryOps is the allowlist of operators accepted in QueryFilter.Op.
	// The operator is interpolated into the SQL query string, so anything not in
	// this list must be rejected to prevent SQL injection.
	allowedQueryOps = map[string]bool{
		"=":     true,
		"<>":    true,
		"!=":    true,
		">":     true,
		"<":     true,
		">=":    true,
		"<=":    true,
		"LIKE":  true,
		"ILIKE": true,
	}
)

const (
	repositorySchema = `
CREATE TABLE IF NOT EXISTS %s (
    id UUID PRIMARY KEY DEFAULT uuidv7(), -- requires Postgres 17+
    version INT NOT NULL DEFAULT 1,
    data JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_%s_data ON %s USING gin (data);
`
)

var RepositorySchema = func(entityName string) string {
	return fmt.Sprintf(repositorySchema, entityName, entityName, entityName)
}

type (
	Entity[T any] struct {
		ID         uuid.UUID
		EntityType string
		Version    int32
		Data       T
	}

	QueryFilter struct {
		Path  string // e.g., "address.city" or "profile.settings.theme"
		Op    string // e.g., "=", ">", "LIKE"
		Value any
	}

	// Pagination bounds the number of rows returned by Query.
	// A nil *Pagination, or a zero-value Pagination, means no limit and no offset.
	Pagination struct {
		Limit  int // maximum number of rows to return; <= 0 means unlimited
		Offset int // number of rows to skip; < 0 is treated as 0
	}

	GenericRepository[T any] struct {
		log        *slog.Logger
		db         *sql.DB
		entityType string
	}
)

// MaxPaginationLimit defines the maximum allowed result size defined by pagination
const MaxPaginationLimit = 10000

// Beware: if T changes its name over time, data will be stored/queried in another entityType
func NewGenericRepository[T any](log *slog.Logger, db *sql.DB) (*GenericRepository[T], error) {
	tType := reflect.TypeFor[T]()
	if tType.Kind() == reflect.Pointer {
		tType = tType.Elem()
	}

	entityTypeName := tType.Name()

	_, err := db.ExecContext(context.Background(), RepositorySchema(entityTypeName))
	if err != nil {
		return nil, err
	}

	return &GenericRepository[T]{
		log:        log.WithGroup("generic").WithGroup(entityTypeName),
		db:         db,
		entityType: entityTypeName,
	}, nil
}

func (r *GenericRepository[T]) Create(ctx context.Context, id uuid.UUID, data T) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Upsert with version initialization or increment
	var query = `
		INSERT INTO ` + r.entityType + ` (id, version, data)
		VALUES ($1, 1, $2)
		ON CONFLICT (id) DO UPDATE 
		SET data = EXCLUDED.data, 
		    version = ` + r.entityType + `.version + 1
	`

	r.log.Debug("create", "id", id, "data", jsonData)

	_, err = r.db.ExecContext(ctx, query, id, jsonData)
	return err
}

func (r *GenericRepository[T]) Update(ctx context.Context, id uuid.UUID, expectedVersion int32, data T) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	var query = `
		UPDATE ` + r.entityType +
		` SET data = $1, version = version + 1 
		WHERE id = $2 AND version = $3
	`

	r.log.Debug("update", "id", id, "version", expectedVersion, "data", jsonData)
	result, err := r.db.ExecContext(ctx, query, jsonData, id, expectedVersion)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		// No row was updated. This happens either because the entity does not
		// exist, or because the expected version does not match the current one.
		// Distinguish the two so callers can handle them separately.
		exists, err := r.exists(ctx, id)
		if err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		return ErrOptimisticLockConflict
	}

	return nil
}

// exists reports whether an entity with the given id exists for this entity type.
func (r *GenericRepository[T]) exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var query = `SELECT EXISTS(SELECT 1 FROM  ` + r.entityType + `  WHERE id = $1)`

	var found bool
	err := r.db.QueryRowContext(ctx, query, id).Scan(&found)
	if err != nil {
		return false, err
	}
	return found, nil
}

// Delete removes the entity with the given id. It returns ErrNotFound if no
// entity matched the id for this entity type.
func (r *GenericRepository[T]) Delete(ctx context.Context, id uuid.UUID) error {
	var query = `DELETE FROM ` + r.entityType + ` WHERE id = $1`

	r.log.Debug("delete", "id", id)
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// Get returns the entity with the given id, or ErrNotFound if no entity matches
// the id for this entity type.
func (r *GenericRepository[T]) Get(ctx context.Context, id uuid.UUID) (*Entity[T], error) {
	var query = `SELECT id, version, data FROM ` + r.entityType + ` WHERE id = $1`

	var (
		ent     Entity[T]
		rawJSON []byte
	)

	r.log.Debug("get", "id", id)
	err := r.db.QueryRowContext(ctx, query, id).Scan(&ent.ID, &ent.Version, &rawJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(rawJSON, &ent.Data); err != nil {
		return nil, err
	}

	return &ent, nil
}

func (r *GenericRepository[T]) Query(ctx context.Context, filters []QueryFilter, pagination *Pagination) ([]Entity[T], error) {
	queryBuilder := strings.Builder{}
	queryBuilder.WriteString("SELECT id, version, data FROM ")
	queryBuilder.WriteString(r.entityType)

	var (
		args        = []any{}
		argIdx      = 1
		firstFilter = true
	)

	for _, f := range filters {
		if !allowedQueryOps[f.Op] {
			return nil, fmt.Errorf("unsupported query operator %q", f.Op)
		}
		if firstFilter {
			// Add WHERE for the first filter only, all following must be AND
			queryBuilder.WriteString(" WHERE")
			firstFilter = false
		} else {
			queryBuilder.WriteString(" AND")
		}

		if f.Op == "=" {
			// Exact equality is expressed with the `@>` containment operator, which the
			// GIN index on `data` accelerates. `#>>` text extraction (below) cannot use it.
			jsonValue, err := jsonPathValue(f.Path, f.Value)
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(&queryBuilder, " data @> $%d", argIdx)
			args = append(args, jsonValue)
			argIdx++
			continue
		}

		// Convert dot notation "profile.address.city" into a Postgres text array representation '{profile,address,city}'
		parts := strings.Split(f.Path, ".")
		jsonPath := "{" + strings.Join(parts, ",") + "}"

		fmt.Fprintf(&queryBuilder, " (data #>> $%d) %s $%d", argIdx, f.Op, argIdx+1)
		args = append(args, jsonPath, f.Value)
		argIdx += 2
	}

	if pagination != nil && pagination.Limit > 0 {
		if pagination.Limit > MaxPaginationLimit {
			return nil, fmt.Errorf("pagination limit must not exceed: %d", MaxPaginationLimit)
		}
		fmt.Fprintf(&queryBuilder, " LIMIT $%d", argIdx)
		args = append(args, pagination.Limit)
		argIdx++
	}

	if pagination != nil && pagination.Offset > 0 {
		fmt.Fprintf(&queryBuilder, " OFFSET $%d", argIdx)
		args = append(args, pagination.Offset)
	}

	r.log.Debug("query", "query", queryBuilder.String(), "args", args)

	rows, err := r.db.QueryContext(ctx, queryBuilder.String(), args...)
	if err != nil {
		return nil, err
	}
	var results []Entity[T]
	for rows.Next() {
		var (
			ent     Entity[T]
			rawJSON []byte
		)
		if err := rows.Scan(&ent.ID, &ent.Version, &rawJSON); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal(rawJSON, &ent.Data); err != nil {
			_ = rows.Close()
			return nil, err
		}
		results = append(results, ent)
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}

	if err := rows.Close(); err != nil {
		return nil, err
	}

	return results, nil
}

// jsonPathValue builds a nested JSON object from a dotted path and a scalar
// value, e.g. ("address.city", "Berlin") -> `{"address":{"city":"Berlin"}}`.
// It is used with the `@>` containment operator so that the GIN index on
// `data` can accelerate exact-equality queries.
func jsonPathValue(path string, value any) (string, error) {
	parts := strings.Split(path, ".")
	var cur = value
	for _, part := range slices.Backward(parts) {
		cur = map[string]any{part: cur}
	}
	b, err := json.Marshal(cur)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
