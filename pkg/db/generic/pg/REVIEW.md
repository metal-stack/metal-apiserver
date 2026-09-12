# Review: `pkg/db/generic/pg/`

Review of the generic Postgres datastore (Entity CRUD, Integer Pool, Shared
Mutex, query filter helpers, RethinkDB→Postgres migration), focused on design
flaws, correctness issues, performance bottlenecks, and maintainability
caveats.

Files reviewed:

- `repository.go`
- `query.go`
- `integer-pool.go`
- `shared-mutex.go`
- `q/machine.go`, `q/network.go`
- `migrations/machine.go`

Status legend: `FIXED` = resolved, `OPEN` = still outstanding, `PARTIAL` =
partially addressed, `NEW` = added in this re-review.

Note: the package is not yet wired into production (only tests and the
migration use it), so design changes are still cheap.

---

## `repository.go`

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

Additional note: `SUM_EQ` and `ARRAY_ELEM_LIKE` run per-row
`jsonb_array_elements` subqueries — O(rows × array length) — and can use no
index at all. Acceptable for small tables (machines), will not scale to larger
entity types.

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

Related caveat: maintaining **two** GIN indexes (`data` + the composite trgm
index) doubles write amplification on every insert/update. Acceptable for now;
revisit if writes become hot.

### 3. `Query` has no result cap / pagination — MEDIUM (performance) — FIXED

`Query(ctx, filters, pagination)` now accepts a `*Pagination` with `Limit` and
`Offset`. A `nil` or zero-value pagination preserves the previous unlimited
behavior. This bounds memory and scan cost for large result sets.

### 4. `Create` vs `Update` race — LOW (documentation) — FIXED

`repository.go` `Create` upserts with `version = 1` on insert and `version + 1` on
conflict. If a `Create` and an `Update` race on the same id, `Create` will
bump the version and clobber `data`. Fine if `Create` is create-only, but worth
documenting the invariant.

### 4a. `Create` is a silent cross-type upsert — HIGH (design/correctness) — FIXED

with no RowsAffected check afterward. So Create on an existing id
silently succeeds and does nothing — no error, no update, no version bump.
The comment above it ("Upsert... or increment") doesn't even match the SQL below it.
This matters beyond style: migrations/machine.go relies on Create for its claimed idempotent-convergence behavior,
but if a machine's data changed in RethinkDB between migration runs,
re-running the migration will not pick up the change — it'll just no-op.
That's a real gap in the migration story that the review doc's "FIXED" label papers over.
I'd treat that whole doc as a snapshot of intent rather than a guarantee of current state.

### 4b. `Get`/`Delete` error semantics differ — LOW (consistency) — FIXED

`Get` now returns `ErrNotFound` for a missing entity, matching `Delete`'s
`ErrNotFound` behavior. Callers must check the error (no longer `(nil, nil)`).

### 4c. `Update` conflates "not found" with "optimistic lock conflict" — MEDIUM (design) — FIXED

`Update` now checks existence when `RowsAffected() == 0` (via a `SELECT EXISTS`)
and returns `ErrNotFound` if the entity is absent, otherwise
`ErrOptimisticLockConflict` for a stale version. Callers can now distinguish the
two cases.

Minor wrinkle: between the failed `UPDATE` and the `SELECT EXISTS`, the entity
can be deleted, in which case a deletion is reported as a lock conflict.

### 4d. `Pagination` has no maximum limit — LOW (design/performance) — FIXED

`Limit` is now rejected above `MaxPaginationLimit` (10000). Remaining caveat:
`Offset` is **unbounded** — `OFFSET 1_000_000_000` scans and discards that many
rows. Cap or clamp `Offset` as well (or move to keyset pagination later).

### 4e. `Query` has no deterministic `ORDER BY` — MEDIUM (correctness) — FIXED

Offset pagination without an `ORDER BY` is non-deterministic: rows can
duplicate or skip between page fetches (concurrent writes shift the scan
order). Add a stable ordering (e.g. `ORDER BY id`).

### 4f. `entityType` derived from the bare Go type name — MEDIUM (design) — NEW

`reflect.TypeFor[T]().Name()` has no package qualification (two `Machine` types
in different packages collide on one partition), and renaming or moving a struct
orphans all of its data (the "Beware" comment acknowledges this).

**Fix:** take an explicit, stable entity-type string (type name as default).

### 4g. DDL executed at construction time — MEDIUM (operational) — NEW

`NewGenericRepository` (and `NewSharedMutex`, `NewIntegerPool`) run
`CREATE TABLE/INDEX/EXTENSION` at startup with `context.Background()`:

- `CREATE EXTENSION pg_trgm` requires a superuser → with a least-privilege DB
  user, repository construction fails outright.
- Every replica runs the DDL concurrently; `CREATE INDEX` takes locks and
  contends across replicas.

**Fix:** move schema management (especially the extension) to migration
tooling; constructors should assume the schema exists.

### 4h. `SUM_EQ` numeric cast aborts the whole query — MEDIUM (robustness) — NEW

`(elem->>$n)::numeric` raises an error if **any** element carries a
non-numeric value at that path, failing the entire query instead of just not
matching. Guard the cast (regex check or `CASE`).

### 4i. `LIKE`/`ILIKE` values are not escaped — LOW (robustness) — NEW

User-provided values containing `%` or `_` act as wildcards. Escape pattern
characters in the value before binding.

### 4j. Debug logs marshal full entity JSON — LOW (security/performance) — NEW

`Create`/`Update`/`Query` log full payloads at debug level
(`repository.go` log calls) — allocation cost on the hot path and sensitive
data (e.g. IPMI credentials) ending up in logs. Log ids/versions only, or redact.

### 4k. No transaction API — MEDIUM (design) — NEW

All entity types share one table, but the repository exposes no
`BeginTx`-style support. Once this backs real entities, multi-entity
operations (allocate machine + touch network/IP) cannot be made atomic. Needed
before wiring into `repository.Store`.

### 7. `rows.Close` error ignored — LOW — FIXED

`repository.go` closes `rows` explicitly after draining and returns the close
error (rather than swallowing it in a deferred `_ = rows.Close()`). Early-return
paths (scan/JSON errors) still close the rows before returning.

---

## `integer-pool.go`

### 5. Lowest-free-id hot-row contention — MEDIUM (performance) — OPEN

`integer-pool.go` acquires `ORDER BY id ASC ... FOR UPDATE SKIP LOCKED`.
The partial index `idx_integer_pool_type_free (pool_type, id) WHERE is_allocated =
FALSE` matches well, but under concurrency every acquirer targets the same lowest
free row, serializing on it. Released low ids also become hot again. This is the
standard "lowest-free" pool pattern; acceptable, but a known bottleneck under
concurrent acquires. Randomized acquisition would spread the contention.

### 5b. `Acquire` exhaustion is not a sentinel error — LOW (design) — FIXED

`Acquire` now returns an `ErrPoolExhausted` sentinel (wrapped with the pool type,
e.g. `fmt.Errorf("%w: pool '%s'", ErrPoolExhausted, poolType)`), so callers can
branch reliably with `errors.Is(err, ErrPoolExhausted)` instead of matching a
formatted string.

### 5c. `id INT` overflows for ASNs — HIGH (correctness) — FIXED

`id INT` is int32 (max 2,147,483,647). Private ASNs are
4,200,000,000–4,294,967,294, and `PoolTypeASN` exists precisely for this.
**Fix:** use `BIGINT`.

### 5d. `Seed` runs the whole range in one statement/transaction — MEDIUM (performance) — NEW

`generate_series` inserts the entire range in a single statement — a huge WAL
spike for wide ranges (the public ASN space alone is ~45M rows). Batch the
inserts.

### 5e. `Release` error is not a sentinel — LOW (consistency) — FIXED

`Release` returns an ad-hoc `fmt.Errorf` ("not found or already released"),
inconsistent with the `ErrPoolExhausted` sentinel; callers cannot
`errors.Is`. Provide distinct sentinels.

---

## `shared-mutex.go`

### 8. `Unlock` has no ownership token — HIGH (correctness) — NEW

`Unlock` deletes by key only. If a holder runs past `expires_at` (default 10s),
the expiration loop removes the row, a second process acquires the lock, and the
first process's `Unlock` then deletes the **new** holder's lock — mutual
exclusion is broken (classic fencing problem).

**Fix:** store a random token/holder id with the row and
`DELETE ... WHERE key = $1 AND token = $2`.

### 9. Expiration computed from the client clock — MEDIUM (correctness) — NEW

`tryLock` uses `time.Now()` on the acquiring host. Clock skew between replicas
distorts when locks expire. Use the server clock (`NOW()`) in the INSERT.

### 10. Waiting acquirers poll in a thundering herd — MEDIUM (performance) — NEW

Each waiter retries every 100ms: N waiters produce ~10N `INSERT` attempts/s
against the primary key. Consider `LISTEN`/`NOTIFY` on release or exponential
backoff.

### 11. Expiration-loop lifetime bound to the constructor `ctx` — MEDIUM (design) — NEW

`NewSharedMutex` starts a goroutine whose lifetime is the `ctx` passed in. If a
caller passes a short-lived (e.g. request-scoped) context, the safety net dies
while locks may still be held. Document the expected lifetime (process-scoped)
or decouple the loop from the argument.

### 12. `Unlock` swallows errors — LOW (robustness) — NEW

Release failures are logged only; the caller cannot detect them, and the next
`Lock` will block until expiration. Consider returning the error.

---

## `query.go` / `q/`

### 6. `PathOf` / `resolveJSONTag` are reflect-heavy and fragile — MEDIUM — PARTIAL

`query.go:77-112` recursively walks the whole struct, comparing `Pointer()`
addresses for **every** selector call, and `ensureInitialized` mutates the `dummy`
value to avoid nil dereferences. For deep nesting this is O(nodes) reflect work per
call.

Partially addressed: `q/machine.go` and `q/network.go` now precompute all paths
once at package init (`machinePaths`/`networkPaths`), so nothing runs in hot
loops. `PathOf` itself remains fragile (pointer-identity matching breaks with
embedded structs/interfaces) and both helpers **panic** on bad input, which
crashes the process during package-init in `q/`. Fail-fast at startup is
defensible, but consider deleting `PathOf` — `SelectorPath` covers the actual
use cases.

### 13. Wrong package doc comment — COSMETIC — FIXED

`q/machine.go:1` says "Package pg provides Postgres query helpers ..." —
copy/paste bug; the package is `q`.

### 14. Hand-written magic-string paths — LOW (maintainability) — NEW

`machinePaths.HardwareCPUsSum` (`"Hardware.MetalCPUs.Cores"`) and
`networkPaths.PrefixIP` (`"Prefixes.IP"`) are literals because `SelectorPath`
cannot traverse slice fields. They are typo-prone and inconsistent with the
reflection-derived paths. Add unit tests asserting they match the actual JSON
output keys. More generally, all current coverage is testcontainers-based
integration (slow, requires Docker in CI); the filter→SQL building logic has no
pure unit tests.

### 15. JSON-tag hazard for metal entities — MEDIUM (design) — NEW

Metal entities have no `json` tags yet (README TODO); paths today equal Go field
names. Adding tags later renames the stored JSON keys, making already-migrated
rows unqueryable unless the data is rewritten. Decide the key-naming strategy
**before** the migration goes live.

### 16. `containsJSON` (q) duplicates `jsonPathValue` (pg) — LOW (maintainability) — NEW

Same wrapping algorithm implemented in two packages. Export one and reuse.

### 17. Filters silently dropped on enum-conversion errors — MEDIUM (behavior) — NEW

`q/machine.go` and `q/network.go` wrap conversions in `if err == nil`
(`enum.GetStringValue`, `metal.ToNATType`): invalid input yields an
**unfiltered** result (returns everything) instead of an error. Surface or log
the error.

---

## `migrations/machine.go`

### 18. Bulk `List` into memory + per-row round trips — LOW (performance) — NEW

`rdb.Machine().List(ctx)` loads every machine into memory, and each
`storeMachine` is its own query (no batching/`COPY`). Fine for a one-shot
migration of a modest fleet; batch if it grows. Partial failures converge on
re-run (documented), but see #4a for the version-drift side effect.

---

## Top recommendations (priority order)

1. **Fix the shared mutex**: ownership token on unlock + server-side expiry
   (#8, #9) — HIGH, correctness
2. **Move DDL out of constructors**, especially `CREATE EXTENSION pg_trgm`
   (#4g) — MEDIUM, operational
3. **Robustness**: `SUM_EQ` cast guard, `LIKE` escaping, stop silently dropping
   filters on conversion errors (#4h, #4i, #17) — MEDIUM
4. **Explicit, stable `entityType` key** (#4f) — MEDIUM
5. **Carry-over open items**: integer-pool hot-row contention (#5), document the
   `Create` vs `Update` invariant (#4), remove/replace `PathOf` (#6)
6. **Before wiring into production**: transaction API (#4k), decide JSON key
   naming (#15)

### Minor / cosmetic

- `Get`/`Delete`/`Update` include `entity_type` in the `WHERE` even though `id` is
  the globally-unique primary key. Harmless (documents type-scoping intent) but
  redundant; note that `id` collisions across entity types are therefore not
  supported.
- `mutexOpt any` / `lockOpt any` option pattern is verbose; plain functional
  options would be simpler.
- `NewGenericRepository` ignores cancellation for its DDL
  (`context.Background()`) — part of #4g.
- `REVIEW.md` itself is a process artifact committed to the repo; consider moving
  accepted findings to issues or deleting after merge.
