package generic

import (
	"context"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/storage"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

const entityAlreadyModifiedErrorMessage = "the entity was already modified, please retry"

type (
	// Entity is the persistence contract an Entity satisfies. It is an alias of
	// the neutral storage.Entity so the RethinkDB implementation and the port
	// share one definition.
	Entity = storage.Entity

	// EntityQuery is the RethinkDB-specific representation of a query. It is an
	// implementation detail of the RethinkDB adapter; callers build it through
	// pkg/db/queries and carry it in a storage.Filter.
	EntityQuery func(q r.Term) r.Term

	// Storage is the backend-agnostic Entity Storage port. It is an alias of
	// storage.Storage; the RethinkDB implementation lives in this package.
	Storage[E Entity] = storage.Storage[E]

	Datastore interface {
		IP() Storage[*metal.IP]
		Machine() Storage[*metal.Machine]
		Size() Storage[*metal.Size]
		SizeImageConstraint() Storage[*metal.SizeImageConstraint]
		SizeReservation() Storage[*metal.SizeReservation]
		Partition() Storage[*metal.Partition]
		Network() Storage[*metal.Network]
		FilesystemLayout() Storage[*metal.FilesystemLayout]
		Image() Storage[*metal.Image]
		Switch() Storage[*metal.Switch]
		SwitchStatus() Storage[*metal.SwitchStatus]
		Event() Storage[*metal.ProvisioningEventContainer]

		// Pools
		AsnPool() *integerPool
		VrfPool() *integerPool

		Lock(ctx context.Context, key string, opts ...lockOpt) error
		Unlock(ctx context.Context, key string, opts ...lockOpt)

		Version(ctx context.Context) (string, error)

		GetTableNames() []string
	}
)
