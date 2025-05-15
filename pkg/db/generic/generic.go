package generic

import (
	"context"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

const entityAlreadyModifiedErrorMessage = "the entity was changed from another, please retry"

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
		// SetChanged sets the entity's changed time
		// TODO: it would be great if SetChanged would be private such that the caller cannot tamper with the modification timestamps
		//       probably this also applies to SetCreated and SetID
		SetChanged(changed time.Time)
		// GetCreated sets the entity's creation time
		GetCreated() time.Time
		// SetCreated sets the entity's creation time
		SetCreated(created time.Time)
	}

	EntityQuery func(q r.Term) r.Term

	Storage[E Entity] interface {
		Create(ctx context.Context, e E) (E, error)
		Update(ctx context.Context, e E) error
		Upsert(ctx context.Context, e E) error
		Delete(ctx context.Context, e E) error
		Get(ctx context.Context, id string) (E, error)
		CheckAlreadyExists(ctx context.Context, id string) error
		Find(ctx context.Context, queries ...EntityQuery) (E, error)
		List(ctx context.Context, queries ...EntityQuery) ([]E, error)
	}

	Datastore interface {
		IP() Storage[*metal.IP]
		Partition() Storage[*metal.Partition]
		Network() Storage[*metal.Network]
		FilesystemLayout() Storage[*metal.FilesystemLayout]
		Image() Storage[*metal.Image]
		// event               Storage[*metal.ProvisioningEventContainer]
		// machine             Storage[*metal.Machine]
		// size                Storage[*metal.Size]
		// sizeimageConstraint Storage[*metal.SizeImageConstraint]
		// sw                  Storage[*metal.Switch]
		// switchStatus        Storage[*metal.SwitchStatus]

		// Pools
		AsnPool() *integerPool
		VrfPool() *integerPool

		Version(ctx context.Context) (string, error)
	}
)
