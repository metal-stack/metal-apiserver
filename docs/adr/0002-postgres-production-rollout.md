# Roll out Postgres with shadow writes, then a single cutover per installation

Every entity can now be served by Postgres (`DB_ROUTING=default=postgres` passes the full test suite), so the open question is how to get there in production gradually. We propose to run the routing layer's `both` mode as a **non-blocking shadow write** for a validation period, with RethinkDB staying the only source of truth, and then switch each installation to Postgres in **one short, coordinated cutover** that moves all entities, the integer pools and the shared mutex together.

The general option catalog (entity-by-entity cutover, dual-write, CDC, read-through, big-bang, sharding) is in [`pkg/db/generic/pg/README.md`](../../pkg/db/generic/pg/README.md). This ADR narrows it down to what fits the code as it stands.

## Status

proposed

## Context

### `both` mode does not work today

The `Router` in `pkg/db/routing` writes to RethinkDB first and mirrors to Postgres. Each backend stamps `created`/`changed` with its own `time.Now()` and keeps its own generation/version. The two copies therefore never agree, and each backend locks on its own copy:

| `both` mode            | before d5154c2               | after d5154c2 (changed-timestamp lock in postgres) |
|------------------------|------------------------------|----------------------------------------------------|
| reads from RethinkDB   | works                        | first update fails with `ErrPartialWrite`          |
| reads from Postgres    | first update fails (Aborted) | first update fails (Aborted)                       |

Reproduced by creating a Network in `both` mode and updating it. RethinkDB additionally stores times with millisecond precision only, so copying the timestamp from one backend to the other is not enough on its own.

Further properties of `both` mode that matter for a rollout:

- A failed mirror write returns `ErrPartialWrite` to the client although RethinkDB already committed it. A retry then conflicts on RethinkDB.
- `read=` is a single global setting in `routing.Config`, not per entity.
- Replicas with different routing configs coexist during a rolling deploy.

### Prerequisites for every strategy

1. **Wire the routing layer into `cmd/server`.** `serve-cmd.go` still builds the RethinkDB datastore with `generic.New` only. Needed: a routing config flag, a Postgres DSN, the `pg.Watcher` (machine Watch), and a Postgres health check.
2. **Data migrations for the remaining entities.** `pkg/db/generic/pg/migrations` only covers Machine and IP. Size, SizeImageConstraint, SizeReservation, Partition, Network, FilesystemLayout, Image, Switch, SwitchStatus and Event are missing.
3. **Integer pools (ASN, VRF) and the shared mutex.** `pg.IntegerPool` and `pg.SharedMutex` exist but are not wired; `routing.Datastore` delegates both to RethinkDB. They cannot move per entity: all replicas must use the same mutex and the same pool, so they need one coordinated switch, plus a migration of the used integers.
4. **Postgres operations:** HA and backups, `random_page_cost = 1.1` (see issue #379), monitoring.

## Decision

Strategy **D: shadow writes, then a single cutover per installation.**

1. **Shadow phase.** Deploy with `default=both,read=rethink`. RethinkDB stays authoritative. The Postgres mirror writes the final state RethinkDB stored (same `created`/`changed`/generation, truncated to RethinkDB's precision) as an upsert without an optimistic lock check, because Postgres is not authoritative yet. A failed mirror write is logged and counted, never returned to the client. This also fixes the regression described above.
2. **Backfill and compare.** A migrate-all command copies every entity and the pool state into Postgres (idempotent upserts). A compare command reports differences between the backends. Run it until the difference stays at zero for the agreed period.
3. **Cutover per installation.** Scale the apiserver to 0, run a final migrate-all and compare, start with `default=postgres` and the Postgres pools and mutex. RethinkDB is kept read-only for rollback.
4. **Decommission** RethinkDB after the rollback period.

## Considered Options

### A. Big-bang per installation

Scale to 0, migrate everything, start with `default=postgres`.

- **Pros:** simplest; `both` mode is never needed; one consistent snapshot; pools and mutex move together.
- **Cons:** downtime (expected to be minutes for metal-stack data sizes); Postgres sees no real traffic before the switch; rollback after the switch loses every write made since.

### B. One entity at a time, each with its own short maintenance window

Rarely-written reference entities first (FilesystemLayout, Image, Size, SizeImageConstraint, Partition), then Network/IP, finally Machine, Event, Switch and SwitchStatus.

- **Pros:** small blast radius; Postgres is proven on easy entities first; no dual write.
- **Cons:** many windows and deploys, each still needing a scale-to-0 because replicas with different configs would split the data; pools and mutex still need their own global switch; rollback per entity loses its writes.

### C. Fully live per entity: `both` → read from Postgres → `postgres`

- **Pros:** no downtime; real traffic validates Postgres; rollback is a config flip while both backends are written.
- **Cons:** the most work and risk:
  - both backends must keep identical timestamps and generations, so the lock problem has to be solved in both directions including the precision difference;
  - partial writes must become log, metric and reconcile instead of client errors;
  - `read=` must become per entity;
  - the backfill races with live writes, so a reconcile tool is required;
  - during rolling deploys, reads must be switched to Postgres (still writing both) before RethinkDB writes are dropped, otherwise replicas still reading RethinkDB see stale data;
  - pools and mutex still need a global switch at the end, so the zero-downtime benefit is mostly lost.

### D. Shadow writes, then a single cutover (chosen)

- **Pros:** validation under real traffic without client-facing risk; the mirror only needs to write RethinkDB's final state, which avoids the two-sided lock problem; all entities, pools and mutex switch together and consistently; rollback before the cutover is free.
- **Cons:** a short maintenance window per installation is still needed; requires a compare/reconcile tool; writes are slightly slower during the shadow phase.

### Comparison

| Strategy                   | Downtime            | Real-traffic validation | Rollback                | Work beyond the prerequisites | Risk of data split |
|----------------------------|---------------------|-------------------------|-------------------------|-------------------------------|--------------------|
| A big-bang                 | minutes, once       | none                    | loses writes            | lowest                        | none               |
| B per-entity windows       | several short ones  | per entity              | loses writes per entity | low                           | low                |
| C fully live               | none                | yes                     | config flip             | highest                       | highest            |
| **D shadow, then cutover** | short, once         | yes                     | free until the cutover  | medium                        | low                |

## Consequences

- `both` mode changes meaning: from a symmetric dual write that is meant to serve reads from either backend, to a one-way, non-blocking mirror. Reading from Postgres in `both` mode is no longer a supported production setting.
- New work items: shadow mirror in `Router` (also fixing the d5154c2 regression), migrate-all and compare commands covering all 12 entities and the pools, wiring in `cmd/server`, Postgres operations.
- Each installation needs one scheduled maintenance window.

## Open questions

- **Other writers to RethinkDB.** If another service (for example the old metal-api) writes to the same RethinkDB tables, mirroring inside the apiserver misses those writes. Then only a cutover after that writer is gone (A, or D's final step) is safe, or the mirror has to come from RethinkDB changefeeds instead (README Option 3).
- Length of the shadow phase and the acceptance criterion for the compare job.
- Whether a reverse migration (Postgres → RethinkDB) is wanted, to make rollback after the cutover possible without losing writes.
