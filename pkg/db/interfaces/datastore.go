package interfaces

import (
	"context"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

// Entity is an interface that allows metal entities to be created and stored
// into the database with the generic creation and update functions.
type Entity interface {
	GetID() string
	SetID(id string)
	GetChanged() time.Time
	GetCreated() time.Time
	GetGeneration() uint64
}

// Storage defines the basic CRUD operations for entities.
// Query types are database-specific and passed as any.
type Storage[E Entity] interface {
	Create(ctx context.Context, e E) (E, error)
	Update(ctx context.Context, e E) error
	Upsert(ctx context.Context, e E) error
	Delete(ctx context.Context, e E) error
	Get(ctx context.Context, id string) (E, error)
	Find(ctx context.Context, queries ...any) (E, error)
	List(ctx context.Context, queries ...any) ([]E, error)
}

// IntegerPool manages unique integers.
type IntegerPool interface {
	AcquireRandomUniqueInteger(ctx context.Context) (uint, error)
	AcquireUniqueInteger(ctx context.Context, value uint) (uint, error)
	ReleaseUniqueInteger(ctx context.Context, id uint) error
}

// Datastore provides access to all entity stores and database-level operations.
type Datastore interface {
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

	AsnPool() IntegerPool
	VrfPool() IntegerPool

	Lock(ctx context.Context, key string, opts ...any) error
	Unlock(ctx context.Context, key string, opts ...any)

	Version(ctx context.Context) (string, error)

	GetTableNames() []string
}
