# Review: `pkg/db/generic/pg/`

Review of the generic Postgres datastore (Entity CRUD + Integer Pool), focused on
correctness issues and performance bottlenecks.

Files reviewed:

- `repository.go`
- `query.go`
- `integer-pool.go`

Status legend: `FIXED` = resolved, `OPEN` = still outstanding, `NEW` = added in
this re-review.

---

## Correctness / bug risks

### 1. SQL injection via the query operator — HIGH — FIXED

`repository.go` validates `f.Op` against the `allowedQueryOps` allowlist before
building the query. Any operator not in `{=, <>, !=, >, <, >=, <=, LIKE, ILIKE}`
is rejected with an error, so the operator can no longer inject arbitrary SQL.

### 2. GIN index is never used by `Query` — HIGH (performance) — FIXED

The equality (`=`) case now uses `data @> $n` with a nested JSON object built
from the path (`jsonPathValue`), which the GIN index on `data` accelerates.
Non-equality operators (`>`, `<`, `LIKE`, ...) still fall back to `#>>` text
extraction, which cannot use the index — so the index now helps the common
exact-match case, but range/LIKE queries remain a table scan. If those are hot,
add a dedicated index (e.g. a `btree` on an extracted path, or a `jsonpath`
expression index) or accept the scan.

### 2b. `@>` filter combined with `entity_type` — MEDIUM (performance) — FIXED

Every query filters on both `entity_type = $1` **and** `data @> $json`. A composite
GIN index is now added to satisfy both predicates from the index:

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX idx_generic_entities_type_data ON generic_entities USING gin (entity_type gin_trgm_ops, data);
```

Note: this requires the `pg_trgm` extension, which needs a superuser to install
(`CREATE EXTENSION`). If that is not acceptable in the target environment, drop
this index — the existing btree index on `entity_type` plus the GIN index on `data`
already let Postgres bitmap-combine the two predicates. The naive composite
`USING gin (entity_type, data)` does **not** work (no default GIN operator class
for `text`), so `gin_trgm_ops` is required here.

### 3. `Query` has no result cap / pagination — MEDIUM (performance) — FIXED

`Query(ctx, filters, pagination)` now accepts a `*Pagination` with `Limit` and
`Offset`. A `nil` or zero-value pagination preserves the previous unlimited
behavior. This bounds memory and scan cost for large result sets.

### 4. `Create` vs `Update` race — LOW (documentation) — OPEN

`repository.go` `Create` upserts with `version = 1` on insert and `version + 1` on
conflict. If a `Create` and an `Update` race on the same id, `Create` will
bump the version and clobber `data`. Fine if `Create` is create-only, but worth
documenting the invariant.

### 4b. `Get`/`Delete` error semantics differ — LOW (consistency) — FIXED

`Get` now returns `ErrNotFound` for a missing entity, matching `Delete`'s
`ErrNotFound` behavior. Callers must check the error (no longer `(nil, nil)`).

### 4c. `Update` conflates "not found" with "optimistic lock conflict" — MEDIUM (design) — FIXED

`Update` now checks existence when `RowsAffected() == 0` (via a `SELECT EXISTS`)
and returns `ErrNotFound` if the entity is absent, otherwise
`ErrOptimisticLockConflict` for a stale version. Callers can now distinguish the
two cases.

### 4d. `Pagination` has no maximum limit — LOW (design/performance) — OPEN

`Pagination.Limit <= 0` means unlimited, but there is no upper bound. A caller
can pass `Limit: 1_000_000_000`, negating the memory/scan bound pagination is
supposed to provide.

**Fix:** clamp `Limit` to a configured maximum (e.g. a repository or package-level
cap) rather than trusting the caller unconditionally.

---

## `integer-pool.go`

### 5. Lowest-free-id hot-row contention — MEDIUM (performance) — OPEN

`integer-pool.go` acquires `ORDER BY id ASC ... FOR UPDATE SKIP LOCKED`.
The partial index `idx_integer_pool_type_free (pool_type, id) WHERE is_allocated =
FALSE` matches well, but under concurrency every acquirer targets the same lowest
free row, serializing on it. Released low ids also become hot again. This is the
standard "lowest-free" pool pattern; acceptable, but a known bottleneck under
concurrent acquires.

### 5b. `Acquire` exhaustion is not a sentinel error — LOW (design) — FIXED

`Acquire` now returns an `ErrPoolExhausted` sentinel (wrapped with the pool type,
e.g. `fmt.Errorf("%w: pool '%s'", ErrPoolExhausted, poolType)`), so callers can
branch reliably with `errors.Is(err, ErrPoolExhausted)` instead of matching a
formatted string.

---

## `query.go`

### 6. `PathOf` / `resolveJSONTag` are reflect-heavy and fragile — MEDIUM — OPEN

`query.go:77-112` recursively walks the whole struct, comparing `Pointer()`
addresses for **every** selector call, and `ensureInitialized` mutates the `dummy`
value to avoid nil dereferences. For deep nesting this is O(nodes) reflect work per
call. If called inside hot loops it is measurable.

**Fix:** prefer `SelectorPath` (cheaper, `query.go:11`) and precompute paths once
at init instead of per query.

### 7. `rows.Close` error ignored — LOW — FIXED

`repository.go` closes `rows` explicitly after draining and returns the close
error (rather than swallowing it in a deferred `_ = rows.Close()`). Early-return
paths (scan/JSON errors) still close the rows before returning.

---

## Top recommendations (priority order)

1. **Precompute selectors** (`SelectorPath` / `PathOf`) outside hot loops; prefer
   `SelectorPath` where possible. (#6) — MEDIUM, OPEN
2. **Accept or redesign the integer-pool hot-row contention** if concurrent
   acquire throughput matters. (#5) — MEDIUM, OPEN
3. **Clamp `Pagination.Limit`** to a configured maximum (#4d). — LOW, OPEN
4. **Document the `Create` vs `Update` invariant** — `Get`/`Delete`/`Update`
   error semantics are now consistent (`ErrNotFound`). (#4) — LOW, OPEN

### Minor / cosmetic

- `Get`/`Delete`/`Update` include `entity_type` in the `WHERE` even though `id` is
  the globally-unique primary key. Harmless (documents type-scoping intent) but
  redundant; note that `id` collisions across entity types are therefore not
  supported.
