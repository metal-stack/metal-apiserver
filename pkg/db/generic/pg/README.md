# Generic Postgres Datastore

Provides two interfaces, one for Entity CRUD and one for Integer Pool.

Main difference of the Entity CRUD compared to what we actually have with rethinkdb:

1. The primary key is `TEXT`, holding the entity's string id verbatim. Both
   UUID-keyed entities (IP, Machine, Network, ...) and named entities (Size,
   Partition, Image, FilesystemLayout, Switch, ...) are supported: UUID entities
   store their UUID string (a UUIDv7 is generated when the id is empty), named
   entities store their meaningful name. No UUID column, no derived/hashed UUID.
   An id only has to be unique within its entity's table (one table per entity
   type). References store the same string id the entity uses, so no
   resolve-by-name indirection is required.

1. Queries must be formatted in a different way

```golang
    machineRepo := pg.NewGenericRepository[Machine](db)
    imagePath := pg.PathOf(func(m *Machine) any {
        return &m.Image.Name
    })
    machines, err := repo.Query(ctx, []pg.QueryFilter{
        {Path: imagePath, Op: "=", Value: "debian-13.0.20260812"},
    })
```

## Migrating from RethinkDB to Postgres

### Why this is not a drop-in swap

The repository layer (`pkg/repository`) does not talk to a storage abstraction
today; it talks to RethinkDB through `generic.Datastore`:

- `Storage[E].Find/List` take an `EntityQuery` (`func(r.Term) r.Term`), so ReQL
  leaks into ~30 repository files and the whole `pkg/db/queries` package.
- `Storage[E]` addresses entities by `string` id and updates by the
  `changed`/`generation` pair, whereas `pg.GenericRepository[T]` uses `string`
  ids, an optimistic `version int32`, and a `QueryFilter`+`Pagination` query
  model.
- RethinkDB has **no multi-document transactions**; the codebase compensates
  with the distributed `SharedMutex`. Postgres has real transactions, so some
  invariants become cheaper — and some cross-store operations become harder
  while data is split.
- Named entities keep their string id as the Postgres primary key (no re-keying
  or lookup-by-name indirection), and JSON keys are now frozen to snake_case.

Any gradual migration therefore has to answer three questions first:

1. **Where is the seam?** (repository → datastore port, or a wrapper around the
   concrete stores)
2. **How is a split across two stores kept consistent?** (dual-write, CDC,
   read-through, or a write-freeze window per entity)
3. **How do ids, concurrency tokens (`generation` vs `version`) and queries map?**

### Options

#### Option 1 — Entity-by-entity backfill + cutover (direct)

Extend the `migrations` package per entity: read from RethinkDB, write to
Postgres, then flip the repository for that entity to Postgres.

- **Pros:** incremental and independently testable/observable per entity; small
  blast radius; no permanent dual-write code; matches what `MigrateMachine`
  already does; low-volume/leaf entities migrate trivially.
- **Cons:** during the transition, entities live in two stores, so
  *cross-entity* references and *multi-entity writes* are no longer atomic;
  a backfill without a write-freeze silently loses concurrent writes; rollback
  after cutover is only possible if RethinkDB was kept current or a reverse
  migration exists.
- **Mitigations:** migrate in dependency order (leaves first); freeze or
  dual-write writes for the entity during its backfill; serialize multi-entity
  writes that span stores with the existing `SharedMutex`.

#### Option 2 — Dual-write / parallel-run wrapper

Implement a `Datastore`/`Storage[E]` wrapper that writes to both stores and reads
from a configurable primary (per entity). RethinkDB stays authoritative, Postgres
is fed in parallel.

- **Pros:** no downtime; instant rollback by flipping the read source; the
  backfill happens as a side effect of normal writes; enables **shadow reads**
  (same query against both stores, diff the results) to validate fidelity before
  trusting Postgres.
- **Cons:** dual-write is not atomic — one store can succeed while the other
  fails, so divergence handling (outbox + retry, idempotent upserts) is required;
  extra write latency and load; the query model still has to be abstracted;
  reads keep paying the RethinkDB cost until the per-entity flip.
- **Useful first step:** *shadow writes* only (write both, read RethinkDB,
  best-effort/async Postgres write) — cheap, no rollback risk, validates the pg
  write path under real traffic.

#### Option 3 — CDC / changefeed replication

Run a sync worker that subscribes to RethinkDB `.changes()` per table and applies
idempotent upserts to Postgres (initial lookup + continuous tail).

- **Pros:** application code untouched; RethinkDB remains the single source of
  truth; sync lag is observable; cut over when lag has been zero for long enough;
  the primitive is already used in this repo (`shared_mutex.go` uses `.Changes`).
- **Cons:** RethinkDB changefeeds are per-table with no global ordering, and
  restarts can miss windows, so exactly-once semantics must be built (ids +
  version/generation guard); deletes need tombstones; still needs a mapping of
  the old document shape to the new JSONB schema; an extra service to operate;
  less trustworthy than a WAL-based CDC.
- **Best fit:** backfilling large, append-heavy tables (events) without a
  write-freeze.

#### Option 4 — Neutral storage port + strangler fig (recommended) — implemented (config toggle)

Introduce a backend-agnostic port (`Storage[E]` with domain semantics, a neutral
filter model, pagination, and the `ErrNotFound`/`ErrAlreadyExists`/
`ErrOptimisticLockConflict` sentinels) and implement it twice: a thin RethinkDB
adapter and a Postgres adapter. Route per entity via configuration, and use
Options 1–3 as the *execution* mechanics behind that seam.

- **Pros:** the only option that cleanly supports per-entity flips (1) *and*
  safe rollback via dual-write (2) *and* optional CDC backfill (3); removes
  `r.Term` from the repository and lets the test/datacenter framework run against
  both backends (already a TODO); each phase is independently shippable.
- **Cons:** upfront refactor of ~30 repository files and the query packages; the
  neutral filter model must cover the subset actually used — the `q/` package is
  a good inventory of that subset — and both backends must implement it.
- **Note:** this is a prerequisite for doing Options 1/2 *safely*, not an
  alternative to them.

The **configuration toggle** is implemented in
[`pkg/db/routing`](../../routing). It is the first slice of this option: every
entity can be served by RethinkDB, Postgres, or both, purely by configuration.

```go
import "github.com/metal-stack/metal-apiserver/pkg/db/routing"

// from a compact string (e.g. read from the environment / deployment config):
//   "default=rethink,read=rethink,machine=both,network=postgres"
cfg, err := routing.Parse(os.Getenv("DB_ROUTING"))

// per entity, build a routed storage from the two adapters
rethinkStorage := routing.NewRethinkStorage(ds.Machine())                       // generic.Storage[*metal.Machine]
postgresStorage := routing.NewPostgresStorage(pgMachineRepo)                    // *pg.GenericRepository[*metal.Machine]
machineStore, err := routing.NewRouter[*metal.Machine](log, cfg, "Machine", rethinkStorage, postgresStorage)
```

Semantics:

| Mode       | Reads                               | Writes                |
|------------|-------------------------------------|-----------------------|
| `rethink`  | RethinkDB                           | RethinkDB             |
| `postgres` | Postgres                            | Postgres              |
| `both`     | `read=` backend (default RethinkDB) | both, RethinkDB first |

- `both` is the transition mode: RethinkDB stays authoritative while Postgres is
  mirrored, so switching the read backend (or rolling back) is a config change.
- A failed mirror write returns `routing.ErrPartialWrite` (the primary write
  already happened, so the backend needs reconciliation).
- Queries carry both representations (`routing.Filter{Rethink: ..., Postgres: ...}`),
  built from the existing `pkg/db/queries` and `pkg/db/generic/pg/q` packages —
  no ReQL→SQL translation. A read requires the filter for the read backend.
  Callers that can build both sides (e.g. the machine repository via
  `routing.FilteredStorage`) use the migration-aware read interface, so a
  Postgres-backed read works even for filtered queries.
- Ids: the entity's string id is used verbatim as the Postgres primary key (the
  column is `TEXT`). UUID-keyed entities that leave their id empty get a
  generated UUIDv7; named entities keep their meaningful name.
- Concurrency: the adapter maps RethinkDB's `generation` to Postgres' `version`
  with the invariant `version = generation + 1`. Under `both`, the Postgres
  mirror is handed a copy of the pre-update entity so the optimistically locked
  version stays correct while the RethinkDB adapter mutates the original.

`routing.Datastore` wires the routers into the existing `generic.Datastore`
interface (per entity, delegating pools/lock/version to RethinkDB), so the
repository and the test harness run on top of it without call-site changes. The
test harness exposes it via `test.WithRoutingConfig(...)`:

```go
s, closer := test.StartRepositoryWithCleanup(t, test.WithRoutingConfig(routing.Config{
    Default:  routing.ModeRethink,
    Entities: map[string]routing.Mode{"Network": routing.ModeBoth},
}))
```

Covered by `pkg/test/routing_test.go` (dual write to both stores, read-back from
RethinkDB, read backend flipped to Postgres, and connect-error mapping).

Not part of this slice (follow-ups): rolling the dual filter representation out
to the remaining entities (currently Machine; the others still build only the
RethinkDB `EntityQuery`), shadow reads / reconciliation, and pagination.

#### Option 5 — Read-through / lazy migration

Read from Postgres; on a miss fall back to RethinkDB and backfill the row.

- **Pros:** no bulk backfill; ideal for cold, low-volume entities; can be added
  per entity with little code.
- **Cons:** cannot distinguish "never migrated" from "genuinely absent/deleted";
  read latency and staleness; entities never read stay in RethinkDB forever, so a
  final backfill is still required before decommissioning; complicates the read
  path for every entity.
- **Best fit:** a complement for rarely-accessed entities, never as the only
  strategy.

#### Option 6 — Big-bang cutover with a maintenance window

One-shot full migration, then deploy the Postgres-only version.

- **Pros:** simplest code and operations; no dual stack, no temporary
  abstractions; no long-lived divergence risk.
- **Cons:** downtime proportional to data size/validation time; no gradual
  validation against real traffic; rollback means restoring the RethinkDB state
  and redeploying — expensive and risky.
- **Best fit:** only if the dataset is small enough for a short window; a
  fallback, not the plan.

#### Option 7 — Shard-by-tenant / per-project migration

Move subsets of data (e.g. a project or partition) at a time so a consistency
cluster moves together.

- **Pros:** preserves referential consistency within a shard; smaller blast
  radius; can run shards in parallel.
- **Cons:** global entities (Partition, Size, Image, FilesystemLayout, shared
  networks) do not shard cleanly; queries need routing or fan-out across stores;
  adds a routing key to every query. High complexity for this schema.

#### Rejected alternative — transliterate ReQL to SQL

Make Postgres satisfy the existing `generic.Storage[E]` interface by translating
the `EntityQuery` (`func(r.Term) r.Term`) closures into SQL. It sounds like a
zero-application-change migration, but it requires reimplementing / interpreting
ReQL at runtime, can only ever cover the subset of terms actually used (and fails
silently on the rest), and produces opaque SQL. The `q/` package exists precisely
because writing the filters twice in a typed way is more maintainable. Rejected:
the neutral port (Option 4) achieves the same "repository doesn't care" outcome
without a query-language interpreter.

### Comparison

| Option                     | Downtime                         | Rollback              | Cross-entity consistency | Validates pg under real traffic | Effort             |
|----------------------------|----------------------------------|-----------------------|--------------------------|---------------------------------|--------------------|
| 1 Entity-by-entity cutover | per-entity freeze, or dual-write | hard after cutover    | at risk while split      | partly                          | medium             |
| 2 Dual-write wrapper       | none                             | easy (flip read flag) | at risk while split      | yes (shadow reads)              | medium/high        |
| 3 CDC replication          | none                             | easy (keep rethink)   | via replication lag      | no (async tail)                 | high (new service) |
| 4 Neutral port + strangler | none                             | easy per entity       | controlled by port       | yes                             | high upfront       |
| 5 Read-through             | none                             | easy                  | at risk while split      | no                              | low/medium         |
| 6 Big-bang                 | yes                              | restore + redeploy    | n/a                      | no                              | low                |
| 7 Shard-by-tenant          | none                             | medium                | preserved per shard      | partly                          | very high          |

### Recommendation

Go with **Option 4 as the seam**, and execute the cutover with the mechanics of
Options 1 and 2 (backfill + shadow-write, then dual-write + per-entity read
flip). Use Option 3 only if a large table (events) must be backfilled without a
write-freeze. Keep Option 5 as a targeted complement for cold entities and
Option 6 as an emergency fallback.

Rationale: the hard part is not copying data, it is keeping cross-entity
invariants while entities are split. A neutral port plus dual-write gives a
one-flag rollback at every step and lets the same repository tests run against
both backends.

### Phased plan

**Phase 0 — Foundation (no behaviour change)**

- Define the port: storage interfaces, neutral filter model, pagination, error
  sentinels. Implement the RethinkDB adapter as a thin delegate to the existing
  `generic` code; make `pkg/repository` depend on the port only.
- Implement id/version mapping: the entity's string id is the primary key
  (`TEXT`); UUID-keyed entities generate a UUIDv7 when empty, named entities keep
  their name. References store the same string id, so no resolve-by-name
  indirection is needed.
- Make the test/datacenter framework run against both adapters
  (the "Adopt Test and Datacenter framework" TODO).

**Phase 1 — Postgres adapter + backfill + shadow writes**

- Implement the port over `pg.GenericRepository` + `q/` filters.
- Backfill entity by entity via `migrations` (Machine done), in dependency order;
  validate with row counts and content checksums.
- Enable shadow writes behind a flag (RethinkDB authoritative, Postgres
  best-effort/async) and a shadow-read comparison job that alerts on diffs.

**Phase 2 — Per-entity read flip (dual-write)**

Suggested order, leaves first:

1. Size, Image, Partition, FilesystemLayout, SizeImageConstraint, SizeReservation
2. Network, IP
3. Machine, Event, Switch, SwitchStatus

- Flip reads per entity via config; keep dual-writes so RethinkDB stays warm.
- Serialize cross-store multi-entity writes with the existing `SharedMutex` until
  all entities involved live in Postgres.
- Exit criteria per entity: shadow-read agreement, error/latency parity, and a
  tested flag-off rollback.

**Phase 3 — Contract (remove RethinkDB)**

- Flip writes to Postgres-only; stop dual-writes; run a final reconciliation.
- Migrate the remaining infrastructure to the Postgres implementations that
  already exist (`pg.SharedMutex`, `pg.IntegerPool`).
- Remove `pkg/db/queries`, the RethinkDB driver/config, and the migration
  tooling after a retention period. Delete the neutral adapter's RethinkDB side.

### Cross-cutting decisions

- **Dual-write failures:** use an outbox (the existing asynq queue/task infra)
  plus idempotent `Upsert` (already bumps `version` only on change).
- **Observability:** per-store latency/error metrics, dual-write divergence
  counters, reconciliation reports; for CDC, sync lag.
- **Deployment config:** per-entity primary/read/write flags, so a rollback is a
  config change, not a release.
- **Postgres operations:** schema DDL (tables/indexes/triggers) currently runs at
  repository construction; move it to migrations (see `REVIEW.md` #4g).
- **Stable keys:** entity types are currently derived from Go type names; switch
  to explicit, stable entity-type strings before the first data is written
  (`REVIEW.md` #4f).

### Rollback strategy

- Phases 0–2: rollback = flip the feature flag back; RethinkDB is still current
  because it was never write-disabled.
- Phase 3: after Postgres-only writes, rollback needs a reverse migration and
  loses writes made in the meantime — so run Phase 3 only after a soak period
  with zero shadow-read mismatches, and keep a tested reverse path for the
  retention window.

## TODO

- [x] add json tags to all properties of the metal entities (snake_case)

### Migration helper

This should be done Entity by Entity

- [x] metal.Machine

### Adopt Test and Datacenter framework

## Claude thoughts on Performance gains

Your results are plausible and line up with what a lot of people have found doing similar RethinkDB→Postgres migrations. Here's why, and what to check to make sure the benchmark itself is trustworthy.

## Why Postgres plausibly wins here

**RethinkDB's architecture works against it for this workload:**

- RethinkDB's storage engine was custom-built and never got the decades of I/O-path optimization Postgres has. Its default *durability=hard* setting does an fsync on every write; a lot of people accidentally benchmark against this instead of `durability=soft`, which makes RethinkDB writes look artificially slow.
- RethinkDB's clustering/consistency layer (Raft-based) adds coordination overhead per write even on a "single node" logical table, since primary/replica bookkeeping still runs.
- Query execution and ReQL evaluation happen through an intermediate query-language layer with real per-query overhead compared to a wire-protocol prepared statement in Postgres.
- Secondary index maintenance in RethinkDB has historically been a weaker/slower path than Postgres's B-tree/GIN maintenance.
- RethinkDB scales less gracefully across cores per shard than Postgres's process-per-connection model backed by decades of multi-core query planner and buffer-manager tuning.

**Postgres's advantages for a k/v-over-JSONB pattern:**

- JSONB is stored as a parsed binary format (not re-parsed text) with fast key lookups; going in through a UUID PK + `WHERE id = $1` is about as simple and cheap a query as Postgres can execute, and it's *extremely* good at that.
- WAL group commit batches concurrent fsyncs efficiently, so with `synchronous_commit` tuned appropriately, throughput on the write path scales well.
- Mature query planner, connection pooling ecosystem (pgbouncer/pgxpool), and prepared statement reuse from the Go driver (`pgx`) all shave overhead RethinkDB's driver stack doesn't have to the same degree.
- Decades of production hardening on MVCC, vacuuming, index types, and TOAST for large JSON blobs.

50k writes/sec on a well-tuned single Postgres instance with pooling is well within normal territory — production Postgres deployments regularly handle far more than that for small-row/point-lookup workloads, so your numbers aren't surprising at all.

## Things to double-check before fully trusting the "5-10x" number

A few common benchmarking pitfalls that could be inflating or deflating the gap:

1. **RethinkDB durability setting** — confirm you're comparing hard-vs-hard or soft-vs-soft (i.e. equivalent `fsync`/`synchronous_commit` guarantees) on both sides. Comparing Postgres async commit against RethinkDB hard durability isn't apples-to-apples.
2. **Connection/session reuse** — is the Go RethinkDB benchmark reusing a connection pool the same way the `pgx` benchmark does? Connection setup overhead per query can dominate.
3. **Batch size / pipelining** — are inserts/updates batched identically (single-row vs multi-row `COPY`/multi-value INSERT) on both sides?
4. **Same hardware, same data set, warm vs cold cache** — make sure both databases had comparable cache warm-up and the same disk (or the same tmpfs/SSD) underneath.
5. **Query equivalence** — is the "query" benchmark doing an indexed point lookup on both sides, or does one benefit from a covering index the other lacks?
6. **JSON (de)serialization cost** — this is Go-side and should be identical unless one path is doing extra marshal/unmarshal round trips.

If those are controlled for, a 5-10x gap is very believable — it matches informal numbers others have reported when moving similar UUID+JSONB "document as row" workloads from RethinkDB to Postgres. I'd suggest running `EXPLAIN (ANALYZE, BUFFERS)` on the Postgres queries and checking `pg_stat_statements` under load to confirm you're getting index-only or single-index-scan plans and not hitting anything unexpected (sequential scans, lock contention, autovacuum stalls) once you scale concurrency up toward your real 50k qps target.
