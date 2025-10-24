package generic

import (
	"context"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

const entityAlreadyModifiedErrorMessage = "the entity was already modified, please retry"

type (
	// Entity is an interface that allows metal entities to be created and stored
	// into the database with the generic creation and update functions.
	Entity interface {
		// GetID returns the entity's id
		GetID() string
		// SetID sets the entity's id
		SetID(id string)
		// GetChanged returns the entity's changed time
		GetChanged() time.Time
		// GetCreated returns the entity's creation time
		GetCreated() time.Time
		// GetGeneration returns the entity's generation
		GetGeneration() uint64
	}

	EntityQuery func(q r.Term) r.Term

	Storage[E Entity] interface {
		Create(ctx context.Context, e E) (E, error)
		Update(ctx context.Context, e E) error
		Upsert(ctx context.Context, e E) error
		Delete(ctx context.Context, e E) error
		Get(ctx context.Context, id string) (E, error)
		Find(ctx context.Context, queries ...EntityQuery) (E, error)
		List(ctx context.Context, queries ...EntityQuery) ([]E, error)
	}

	Datastore interface {
		IP() Storage[*metal.IP]
		Machine() Storage[*metal.Machine]
		Size() Storage[*metal.Size]
		Partition() Storage[*metal.Partition]
		Network() Storage[*metal.Network]
		FilesystemLayout() Storage[*metal.FilesystemLayout]
		Image() Storage[*metal.Image]
		Switch() Storage[*metal.Switch]
		SwitchStatus() Storage[*metal.SwitchStatus]
		Event() Storage[*metal.ProvisioningEventContainer]

		// sizeimageConstraint Storage[*metal.SizeImageConstraint]
		// sw                  Storage[*metal.Switch]
		// switchStatus        Storage[*metal.SwitchStatus]

		// Pools
		AsnPool() *integerPool
		VrfPool() *integerPool

		Version(ctx context.Context) (string, error)
	}
)
