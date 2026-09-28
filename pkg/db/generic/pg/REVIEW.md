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

### 4a. `Create` silently skips on conflict — HIGH (design/correctness) — FIXED

`Create` now checks `RowsAffected` and returns an `ErrAlreadyExists` sentinel
when the id already exists (same **or different** entity type); the existing row
is left untouched. Callers can distinguish "created" from "already present".

For the migration story, a new `Upsert` method was added: it inserts or replaces
the data for the same entity type, bumps `version` only when the data actually
changed (`IS DISTINCT FROM`), and returns `ErrAlreadyExists` if the id is owned
by a different entity type. `migrations/machine.go` now uses `Upsert`, so
re-running the migration converges machines whose data changed in RethinkDB in
between, while unchanged machines keep their version. Covered by
`TestGenericRepositoryUpsert` and the "converges changed data on re-run"
migration subtest.

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

### 4l. `DEFAULT uuidv7()` version claim and dead default — LOW (maintainability) — NEW

The schema comment says "requires Postgres 18+", but verify which release
actually ships `uuidv7()` (tests run against `postgres:19beta3-alpine`, a beta
image). Also note the default is effectively dead code: every write path
supplies an explicit id, so the `DEFAULT` only matters for out-of-band inserts.

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

Note: with the range-counter redesign, growth contention has moved to the single
`integer_pool_state` row — every grower serializes on that one `UPDATE`. It is
much cheaper than scanning the pool table, but it is still a hot spot at very
high acquire concurrency.

### 5b. `Acquire` exhaustion is not a sentinel error — LOW (design) — FIXED

`Acquire` now returns an `ErrPoolExhausted` sentinel (wrapped with the pool type,
e.g. `fmt.Errorf("%w: pool '%s'", ErrPoolExhausted, poolType)`), so callers can
branch reliably with `errors.Is(err, ErrPoolExhausted)` instead of matching a
formatted string.

### 5c. `id INT` overflows for ASNs — HIGH (correctness) — FIXED

`id INT` is int32 (max 2,147,483,647). Private ASNs are
4,200,000,000–4,294,967,294, and `PoolTypeASN` exists precisely for this.
**Fix:** use `BIGINT`.

### 5d. `Seed` runs the whole range in one statement/transaction — MEDIUM (performance) — FIXED

Resolved by the range-counter redesign: `Seed` now only records the bounds in
`integer_pool_state` and the pool grows lazily on demand — no `generate_series`,
no WAL spike. See #19–#24 for issues introduced by the new design.

### 5e. `Release` error is not a sentinel — LOW (consistency) — FIXED

`Release` now wraps the `ErrIntegerNotFound` sentinel, so callers can
`errors.Is`.

### 19. `Seed` rewinds the growth counter — HIGH (correctness) — FIXED

`Seed` used `next = LEAST(integer_pool_state.next, EXCLUDED.next)`, so re-seeding
a pool with a lower start (or re-running seed logic after growth) rewound `next`
and `Acquire` could hand out already-allocated integers.

`Seed` is now insert-once: it uses `ON CONFLICT (pool_type) DO NOTHING` and
returns `ErrPoolAlreadySeeded` if the pool already exists, so the growth counter
can never move backwards. Covered by the "re-seeding an already seeded pool must
fail" assertion in `TestIntegerPoolService`.

`Seed` now also rejects an inverted range (`startID > endID`). Note: this makes
`Seed` non-idempotent by design — startup code that seeds unconditionally must
tolerate `ErrPoolAlreadySeeded` (or check first).

### 20. `Acquire` grow + record is not transactional — MEDIUM (robustness) — NEW

The counter increment (`UPDATE ... RETURNING next - 1`) and the `record` insert
are separate statements with no surrounding transaction. If the process dies or
the `record` fails in between, the integer is lost forever: the counter has
moved past it but no pool row exists, so it can never be acquired or released.
**Fix:** wrap both statements in one transaction.

### 21. `AcquireUniqueInteger` ignores the lower bound — MEDIUM (correctness) — NEW

Only `value <= max` is checked; the error message even claims the range is
"0 - max". The pool's start is stored in `next`, which moves as the pool grows,
so the original lower bound is no longer queryable. A pool seeded with
`start > 0` accepts unique acquires below its configured range.
**Fix:** store `min` in `integer_pool_state` and check `min <= value <= max`.

### 22. `AcquireUniqueInteger` reports unseeded pools as exhausted — LOW (behavior) — NEW

A missing `integer_pool_state` row (pool never seeded) returns
`ErrPoolExhausted`, which is the wrong diagnosis. Add an
`ErrPoolNotSeeded`-style sentinel.

### 23. `AcquireUniqueInteger` takes two round trips where one atomic statement suffices — LOW (performance) — NEW

`claim` (UPDATE) then `insert` (INSERT ... ON CONFLICT DO NOTHING) can be a
single statement:

```sql
INSERT INTO integer_pool (pool_type, id, is_allocated) VALUES ($1, $2, TRUE)
ON CONFLICT (pool_type, id) DO UPDATE SET is_allocated = TRUE
WHERE integer_pool.is_allocated = FALSE
RETURNING id;
```

One round trip, same atomicity (0 rows ⇒ already allocated).

### 24. `BIGINT` column vs `uint32` Go API — LOW (consistency) — FIXED

The schema uses `BIGINT` but every Go signature takes/returns `uint32`. Scanning
a `BIGINT` into `uint32` errors out-of-range, so the wider column buys nothing
today and invites confusion. Pick one: `int64`/`BIGINT` end to end, or
`INTEGER`/`uint32` (which still covers the full ASN space).

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

### 15. JSON-tag hazard for metal entities — MEDIUM (design) — FIXED

Decided before the migration went live: every exported struct field in
`pkg/db/metal` now carries an explicit **snake_case** `json` tag (e.g.
`PartitionID` → `partition_id`, `MachineNetworks` → `machine_networks`,
`MetalCPUs` → `metal_cpus`). RethinkDB is unaffected (its driver uses only the
`rethinkdb` tag), and the Postgres storage keys are now stable and independent
of later Go renames. `SelectorPath` picks the names up automatically, so
`q.MachineFilter`/`q.NetworkFilter` produce snake_case paths; the two paths that
could not be derived by reflection (`hardware.metal_cpus.cores`,
`prefixes.ip`) and the `@>`-containment element keys were updated by hand.

Going forward: any new field must ship with a `json` tag, otherwise the stored
key silently follows the Go field name.

### 16. `containsJSON` (q) duplicates `jsonPathValue` (pg) — LOW (maintainability) — NEW

Same wrapping algorithm implemented in two packages. Export one and reuse.

### 17. Filters silently dropped on enum-conversion errors — MEDIUM (behavior) — NEW

`q/machine.go` and `q/network.go` wrap conversions in `if err == nil`
(`enum.GetStringValue`, `metal.ToNATType`): invalid input yields an
**unfiltered** result (returns everything) instead of an error. Surface or log
the error.

### 26. `netip.MustParsePrefix` panics on malformed query input — MEDIUM (robustness) — NEW

`q/network.go` (`NetworkFilter`) calls `netip.MustParsePrefix` on
`rq.Prefixes` / `rq.DestinationPrefixes` — values that originate from API
queries. A malformed prefix panics in the request path instead of returning an
error. Use `netip.ParsePrefix` and fail (or skip with a logged warning) on
error.

---

## `migrations/machine.go`

### 18. Bulk `List` into memory + per-row round trips — LOW (performance) — NEW

`rdb.Machine().List(ctx)` loads every machine into memory, and each
`storeMachine` is its own query (no batching/`COPY`). Fine for a one-shot
migration of a modest fleet; batch if it grows. Re-runs converge changed data
and leave the version of unchanged entities alone (see #4a, `Upsert`).

---

## Top recommendations (priority order)

1. **Fix the shared mutex**: ownership token on unlock + server-side expiry
   (#8, #9) — HIGH, correctness
2. ~~Stop `Seed` from rewinding the growth counter~~ — done: `Seed` is
   insert-once and returns `ErrPoolAlreadySeeded` (#19)
3. ~~Give `Create` a conflict result~~ — done: `ErrAlreadyExists` + `Upsert`
   with data-change-only version bump; migration converges on re-run (#4a)
4. **Make unique-acquire range checks real**: store and check the lower bound
   (#21), wrap grow+record in a transaction (#20) — MEDIUM, correctness
5. **Move DDL out of constructors**, especially `CREATE EXTENSION pg_trgm`
   (#4g) — MEDIUM, operational
6. **Robustness batch**: `SUM_EQ` cast guard, `LIKE` escaping, no silent filter
   drops, no `MustParsePrefix` panic (#4h, #4i, #17, #26) — MEDIUM
7. **Explicit, stable `entityType` key** (#4f) — MEDIUM
8. **Carry-over open items**: pool hot-row/counter-row contention (#5),
   `PathOf` removal (#6), unbounded `Offset` (#4d)
9. **Before wiring into production**: transaction API (#4k), decide JSON key
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
