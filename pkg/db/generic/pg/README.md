# Generic Postgres Datastore

Provides two interfaces, one for Entity CRUD and one for Integer Pool.

Main difference of the Entity CRUD compared to what we actually have with rethinkdb:

1. ID must be a UUIDv7, this will hurt for Entities which actually have a non-UUID id:

- IP
- Size
- Partition
- Image
- Filesystemlayout

These Entities must get a UUIDv7 during migration and Queries must be adopted to search by the Name property of it.
As these Entities are low volume, this should not hurt performance.
Also Name Uniqueness must also be ensured on repository layer, or newly created machines must reference them by uuid instead ?
Could be made possible by checking if the reference is a uuid, otherwise query by name.

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

## TODO

- [ ] add json tags to all properties of the metal entities

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