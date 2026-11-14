# Extract the Entity Storage port into pkg/db/storage

The neutral Entity Storage port (Entity, Filter and Storage) lives in `pkg/db/storage`, not in `pkg/db/routing`. `pkg/db/generic`'s Datastore accessors return `storage.Storage[E]`, `pkg/db/generic`'s `Entity`/`Storage` become aliases of the neutral port, every repository read goes through the single port (`Find`/`List` with `storage.Filter`), and `pkg/db/routing` shrinks to the Postgres adapter plus the configuration-driven `Router`. The RethinkDB port implementation is the existing `rethinkStorage` in `pkg/db/generic`, so `routing` no longer needs a RethinkDB adapter of its own.

The four overlapping read shapes (`generic.Storage.Find/List`, `routing.Storage.Find/List`, `routing.FilteredStorage.FindFiltered/ListFiltered`, and the type-asserting `routing.Find/List`) collapsed into one: `storage.Storage[E].Find/List(ctx, ...storage.Filter)`.

We extracted a dedicated package because `pkg/db/routing` already imports `pkg/db/generic`, while the Repository holds `generic.Datastore`; the port could therefore never be referenced from `generic` without an import cycle. Keeping the port in `routing` was the obvious alternative, but it would have forced every Repository file and the test harness to depend on the routing module concretely, and it could not fix the cycle. The cost is one more package and an `any`-typed filter payload at the seam; the win is that "which Backend serves this Entity" is answered in exactly one place for every Entity, and each entity's read flip becomes a configuration change.

## Error mapping lives in the adapters, not the orchestrator

The port's error contract for storage outcomes (not-found, already-exists, lock conflict) is connect errors with the entity's legacy name — the same contract `generic.Storage` always had. Each backend adapter maps its own error vocabulary directly (`postgresStorage` maps `pg.ErrNotFound`/`pg.ErrAlreadyExists`/`pg.ErrOptimisticLockConflict`; the RethinkDB storage already returned connect errors), so no separate decorator or neutral sentinel layer is needed for those outcomes. The one retained sentinel, `storage.ErrFilterUnsupported`, is not a storage outcome but a filter-representation error: it means a caller asked a backend to serve a read whose filter carries no representation for it.

The original plan put neutral sentinels at the port and moved the sentinel→connect mapping into the `store[R,E,...]` orchestrator. Investigation showed that is wrong: the repository surfaces storage errors from far more than the orchestrator — validation helpers (`validateCreate/Update/Delete`) and every `AdditionalMethods` read call the entity storage directly (172 `r.s.ds.` call sites). Mapping only in the orchestrator would have turned a `NotFound` from a validation read into an `InvalidArgument` (validation errors are wrapped with `WrapConnectErr(InvalidArgument, …)`) and leaked sentinels from `AdditionalMethods`. A neutral sentinel layer would also need a decorator on every datastore accessor to reach those callers, and `routing` needs the un-decorated port to build its `Router`. Mapping atomically in the adapter is the single high-locality place that covers all callers.

## Status

accepted

## Considered Options

- **Keep the port in `pkg/db/routing`** and have the Repository hold a concrete `*routing.Datastore`. Rejected: couples every Repository file to the routing module and leaves the four overlapping read shapes in place.
- **Put the port in `pkg/db/generic` itself** and let `routing` be the adapter layer. Rejected: `generic` would then own both the neutral port and its legacy ReQL storage, blurring the seam.
- **Leave `generic.Datastore` untouched and add a parallel read-only port.** Rejected: two handles for the same concern defeats the goal of a single read interface.
- **Map sentinels to connect errors in the `store[R,E,...]` orchestrator.** Rejected as described above: it does not cover validation helpers or `AdditionalMethods`, and changes their error semantics.
