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
)

var (
	ErrOptimisticLockConflict = errors.New("optimistic lock conflict: entity has been modified")

	ErrNotFound = errors.New("entity not found")

	// ErrAlreadyExists is returned by Create when an entity with the same id
	// already exists in the entity's table.
	ErrAlreadyExists = errors.New("entity already exists")

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
-- One table per entity type. The primary key is a text id: UUID-keyed
-- entities store their UUID string, named entities (partition, size, image, ...)
-- store their meaningful name. See Entity.ID.
CREATE TABLE IF NOT EXISTS %s (
    id TEXT PRIMARY KEY,
    version INT NOT NULL DEFAULT 1,
    data JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_%s_data ON %s USING gin (data);

-- One shared trigger function serves every per-entity table: the table name
-- is the entity type, so it is read from TG_TABLE_NAME rather than from a
-- trigger argument that would have to be kept in sync with the table.
CREATE OR REPLACE FUNCTION entity_table_notify_change() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify(
        'entity_table_changes',
        json_build_object(
            'id', (COALESCE(NEW.id, OLD.id))::text,
            'entity_type', TG_TABLE_NAME,
            'op', TG_OP
        )::text
    );
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS entity_table_notify_change ON %s;
CREATE TRIGGER entity_table_notify_change
    AFTER INSERT OR UPDATE OR DELETE ON %s
    FOR EACH ROW EXECUTE FUNCTION entity_table_notify_change();
`
)

var RepositorySchema = func(entityName string) string {
	return fmt.Sprintf(repositorySchema, entityName, entityName, entityName, entityName, entityName)
}

type (
	Entity[T any] struct {
		ID         string
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
		watcher    *Watcher
	}

	// RepositoryOption customizes how a GenericRepository is built.
	RepositoryOption func(*repositoryOptions)

	repositoryOptions struct {
		watcher *Watcher
	}
)

// WithWatcher attaches a PostgreSQL watcher to the repository, enabling the
// Watch method. A single Watcher can be shared by every repository that uses
// the same database. Without a watcher, Watch returns ErrWatchNotConfigured.
func WithWatcher(w *Watcher) RepositoryOption {
	return func(o *repositoryOptions) {
		o.watcher = w
	}
}

// MaxPaginationLimit defines the maximum allowed result size defined by pagination
const MaxPaginationLimit = 10000

// Beware: if T changes its name over time, data will be stored/queried in another entityType
func NewGenericRepository[T any](log *slog.Logger, db *sql.DB, opts ...RepositoryOption) (*GenericRepository[T], error) {
	tType := reflect.TypeFor[T]()
	if tType.Kind() == reflect.Pointer {
		tType = tType.Elem()
	}

	// Postgres folds unquoted identifiers to lowercase, so the table name -
	// and thus the entity type announced by the trigger (TG_TABLE_NAME) - is
	// the lowercased Go type name.
	entityTypeName := strings.ToLower(tType.Name())

	options := &repositoryOptions{}
	for _, opt := range opts {
		opt(options)
	}

	_, err := db.ExecContext(context.Background(), RepositorySchema(entityTypeName))
	if err != nil {
		return nil, err
	}

	return &GenericRepository[T]{
		log:        log.WithGroup("generic").WithGroup(entityTypeName),
		db:         db,
		entityType: entityTypeName,
		watcher:    options.watcher,
	}, nil
}

// Create inserts a new entity. It returns ErrAlreadyExists if an entity with
// the same id already exists; the existing row is left untouched.
func (r *GenericRepository[T]) Create(ctx context.Context, id string, data T) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	var query = `
		INSERT INTO ` + r.entityType + ` (id, version, data)
		VALUES ($1, 1, $2)
		ON CONFLICT (id) DO NOTHING
	`

	r.log.Debug("create", "id", id, "data", jsonData)

	result, err := r.db.ExecContext(ctx, query, id, jsonData)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("%w: id %s", ErrAlreadyExists, id)
	}

	return nil
}

// Upsert inserts the entity or replaces the data of the existing entity with
// the same id. The version is bumped only when the data actually changes, so
// upserting identical data is a no-op.
func (r *GenericRepository[T]) Upsert(ctx context.Context, id string, data T) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	var query = `
		INSERT INTO ` + r.entityType + ` (id, version, data)
		VALUES ($1, 1, $2)
		ON CONFLICT (id) DO UPDATE
		SET data = EXCLUDED.data,
		    version = CASE WHEN ` + r.entityType + `.data IS DISTINCT FROM EXCLUDED.data
		                  THEN ` + r.entityType + `.version + 1
		                  ELSE ` + r.entityType + `.version END
	`

	r.log.Debug("upsert", "id", id, "data", jsonData)

	_, err = r.db.ExecContext(ctx, query, id, jsonData)
	return err
}

func (r *GenericRepository[T]) Update(ctx context.Context, id string, expectedVersion int32, data T) error {
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
func (r *GenericRepository[T]) exists(ctx context.Context, id string) (bool, error) {
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
func (r *GenericRepository[T]) Delete(ctx context.Context, id string) error {
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
func (r *GenericRepository[T]) Get(ctx context.Context, id string) (*Entity[T], error) {
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

// Watch streams changes of the entity with the given id. The returned channel
// yields the old and the new value of every change and is closed when ctx is
// cancelled or the watcher is closed.
//
// Changes are delivered through native PostgreSQL NOTIFY/LISTEN: the per-entity
// table carries a trigger that announces the id of every changed row, the
// Watcher loads the committed row and the repository decodes it into T. The old
// value is the value observed before the change, so the first change of an
// entity has a zero Old value and a deletion has a zero New value.
func (r *GenericRepository[T]) Watch(ctx context.Context, id string) (<-chan struct {
	Old T
	New T
}, error) {
	if r.watcher == nil {
		return nil, ErrWatchNotConfigured
	}

	changes, err := r.watcher.Subscribe(ctx, r.entityType, id)
	if err != nil {
		return nil, err
	}

	results := make(chan struct {
		Old T
		New T
	})

	go func() {
		defer close(results)

		var previous T

		for {
			select {
			case <-ctx.Done():
				return
			case change, ok := <-changes:
				if !ok {
					return
				}

				var pair struct {
					Old T
					New T
				}

				if len(change.New) > 0 {
					var current T
					if err := json.Unmarshal(change.New, &current); err != nil {
						r.log.Error("unable to decode changed entity", "id", id, "error", err)
						continue
					}
					pair.Old = previous
					pair.New = current
					previous = current
				} else {
					// A deletion carries no new state; the previously observed
					// value becomes the old value.
					pair.Old = previous
					var zero T
					previous = zero
				}

				select {
				case results <- pair:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return results, nil
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
		if firstFilter {
			// Add WHERE for the first filter only, all following must be AND
			queryBuilder.WriteString(" WHERE")
			firstFilter = false
		} else {
			queryBuilder.WriteString(" AND")
		}

		switch f.Op {
		case "=":
			// Exact equality is expressed with the `@>` containment operator, which the
			// GIN index on `data` accelerates. `#>>` text extraction (below) cannot use it.
			jsonValue, err := jsonPathValue(f.Path, f.Value)
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(&queryBuilder, " data @> $%d", argIdx)
			args = append(args, jsonValue)
			argIdx++

		case "@>":
			// Direct JSON containment with a caller-provided structured value (map/slice).
			// Unlike `=`, the value is used verbatim instead of being wrapped at the
			// given path. This expresses membership inside nested arrays and maps,
			// e.g. `{"Hardware":{"Nics":[{"MacAddress":"aa:bb"}]}}` matches any machine
			// whose nics array contains an element with that mac. It uses the same
			// GIN-indexable `@>` operator as `=`.
			jsonValue, err := json.Marshal(f.Value)
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(&queryBuilder, " data @> $%d", argIdx)
			args = append(args, string(jsonValue))
			argIdx++

		case "IS NULL", "IS NOT NULL":
			// Presence check: `data #>> path` returns NULL when the key (or any
			// ancestor key) is absent, so this expresses "field is present/absent".
			var (
				parts       = strings.Split(f.Path, ".")
				jsonPath    = "{" + strings.Join(parts, ",") + "}"
				nullKeyword = "NULL"
			)
			if f.Op == "IS NOT NULL" {
				nullKeyword = "NOT NULL"
			}
			fmt.Fprintf(&queryBuilder, " (data #>> $%d) IS %s", argIdx, nullKeyword)
			args = append(args, jsonPath)
			argIdx++

		case "SUM_EQ":
			// Sum of a numeric field across an array of objects equals the value.
			// The path's last segment is the numeric field within each element; the
			// preceding segments identify the array, e.g. "Hardware.MetalCPUs.Cores"
			// sums the `Cores` field over data.Hardware.MetalCPUs.
			//
			// The `jsonb_typeof` guard treats a missing/`null`/scalar value at the
			// array path as an empty array so that `jsonb_array_elements` never
			// errors on a non-array value (a nil slice marshals to JSON `null`).
			var (
				parts    = strings.Split(f.Path, ".")
				field    = parts[len(parts)-1]
				arrParts = parts[:len(parts)-1]
				arrPath  = "{" + strings.Join(arrParts, ",") + "}"
			)
			fmt.Fprintf(&queryBuilder, " (SELECT COALESCE(SUM((elem->>$%d)::numeric),0) FROM jsonb_array_elements(CASE WHEN jsonb_typeof(data #> $%d) = 'array' THEN data #> $%d ELSE '[]'::jsonb END) AS elem) = $%d", argIdx, argIdx+1, argIdx+1, argIdx+2)
			args = append(args, field, arrPath, f.Value)
			argIdx += 3

		case "ARRAY_ELEM_LIKE":
			// Match if any element of an array of objects has the given field
			// matching a LIKE pattern. The path's last segment is the field within
			// each element; the preceding segments identify the array, e.g.
			// "Prefixes.IP" checks the `IP` field of every element in data.Prefixes.
			//
			// The `jsonb_typeof` guard treats a missing/`null`/scalar value at the
			// array path as an empty array, so the EXISTS never errors.
			// Note: the value is an already-formatted LIKE pattern (e.g. the
			// address-family checks in q/network.go pass `%.%` / `%:%`), so it
			// is intentionally not escaped. User-provided values must go through
			// the LIKE/ILIKE operators instead, which are escaped below.
			var (
				parts    = strings.Split(f.Path, ".")
				field    = parts[len(parts)-1]
				arrParts = parts[:len(parts)-1]
				arrPath  = "{" + strings.Join(arrParts, ",") + "}"
			)
			fmt.Fprintf(&queryBuilder, " EXISTS (SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(data #> $%d) = 'array' THEN data #> $%d ELSE '[]'::jsonb END) AS elem WHERE elem->>$%d LIKE $%d)", argIdx, argIdx, argIdx+1, argIdx+2)
			args = append(args, arrPath, field, f.Value)
			argIdx += 3

		default:
			if !allowedQueryOps[f.Op] {
				return nil, fmt.Errorf("unsupported query operator %q", f.Op)
			}

			// Convert dot notation "profile.address.city" into a Postgres text array representation '{profile,address,city}'
			var (
				parts    = strings.Split(f.Path, ".")
				jsonPath = "{" + strings.Join(parts, ",") + "}"
			)

			// LIKE/ILIKE treat %, _ and the escape character as pattern
			// metacharacters. Escape them in the caller-provided value so a
			// user-supplied string is matched literally instead of acting as a
			// wildcard pattern.
			var (
				value  = f.Value
				escape = ""
			)
			if f.Op == "LIKE" || f.Op == "ILIKE" {
				if s, ok := value.(string); ok {
					value = escapeLikePattern(s)
					escape = ` ESCAPE '\'`
				}
			}

			fmt.Fprintf(&queryBuilder, " (data #>> $%d) %s $%d%s", argIdx, f.Op, argIdx+1, escape)
			args = append(args, jsonPath, value)
			argIdx += 2
		}
	}

	fmt.Fprintf(&queryBuilder, " ORDER BY id")

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

// escapeLikePattern escapes the LIKE/ILIKE metacharacters `%`, `_` and the
// escape character `\` in a caller-provided value. Combined with the
// `ESCAPE '\'` clause this makes the value match literally, so user input
// cannot smuggle in wildcards (REVIEW.md #4i).
func escapeLikePattern(s string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	).Replace(s)
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
