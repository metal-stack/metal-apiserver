package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/metal-stack/metal-apiserver/pkg/db/interfaces"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

type datastore struct {
	log    *slog.Logger
	pool   *pgxpool.Pool
	dbname string

	ip                  *storage[*metal.IP]
	machine             *storage[*metal.Machine]
	event               *storage[*metal.ProvisioningEventContainer]
	size                *storage[*metal.Size]
	sizeImageConstraint *storage[*metal.SizeImageConstraint]
	sizeReservation     *storage[*metal.SizeReservation]
	partition           *storage[*metal.Partition]
	network             *storage[*metal.Network]
	fsl                 *storage[*metal.FilesystemLayout]
	image               *storage[*metal.Image]
	sw                  *storage[*metal.Switch]
	switchStatus        *storage[*metal.SwitchStatus]

	asnPool *integerPool
	vrfPool *integerPool

	locker *pgLocker

	tableNames []string
}

var _ interfaces.Datastore = (*datastore)(nil)

func New(log *slog.Logger, cfg Config) (interfaces.Datastore, error) {
	log = log.WithGroup("datastore")

	log.Info("create postgres client")

	ds := &datastore{
		log:  log,
		pool: cfg.Pool,
	}

	ds.ip = newStorage[*metal.IP](ds, "ip")
	ds.machine = newStorage[*metal.Machine](ds, "machine")
	ds.size = newStorage[*metal.Size](ds, "size")
	ds.sizeImageConstraint = newStorage[*metal.SizeImageConstraint](ds, "sizeimageconstraint")
	ds.sizeReservation = newStorage[*metal.SizeReservation](ds, "sizereservation")
	ds.partition = newStorage[*metal.Partition](ds, "partition")
	ds.network = newStorage[*metal.Network](ds, "network")
	ds.fsl = newStorage[*metal.FilesystemLayout](ds, "filesystemlayout")
	ds.image = newStorage[*metal.Image](ds, "image")
	ds.event = newStorage[*metal.ProvisioningEventContainer](ds, "event")
	ds.sw = newStorage[*metal.Switch](ds, "switch")
	ds.switchStatus = newStorage[*metal.SwitchStatus](ds, "switchstatus")

	var (
		vrfMin  = uint(1)
		vrfMax  = uint(131072)
		asnMin  = uint(1)
		asnMax  = uint(131072)
	)

	ds.asnPool = newIntegerPool(ds, "asnpool", asnMin, asnMax)
	ds.vrfPool = newIntegerPool(ds, "vrfpool", vrfMin, vrfMax)

	ds.locker = newPgLocker(log, cfg.Pool)

	return ds, nil
}

func (ds *datastore) Version(ctx context.Context) (string, error) {
	var version string
	err := ds.pool.QueryRow(ctx, "SELECT version()").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("cannot query postgres version: %w", err)
	}
	return version, nil
}

func (ds *datastore) Lock(ctx context.Context, key string, opts ...any) error {
	return ds.locker.lock(ctx, key, opts...)
}

func (ds *datastore) Unlock(ctx context.Context, key string, opts ...any) {
	ds.locker.unlock(ctx, key)
}

func (ds *datastore) IP() interfaces.Storage[*metal.IP]                    { return ds.ip }
func (ds *datastore) Machine() interfaces.Storage[*metal.Machine]          { return ds.machine }
func (ds *datastore) Size() interfaces.Storage[*metal.Size]                { return ds.size }
func (ds *datastore) SizeImageConstraint() interfaces.Storage[*metal.SizeImageConstraint] { return ds.sizeImageConstraint }
func (ds *datastore) SizeReservation() interfaces.Storage[*metal.SizeReservation] { return ds.sizeReservation }
func (ds *datastore) Partition() interfaces.Storage[*metal.Partition]      { return ds.partition }
func (ds *datastore) Network() interfaces.Storage[*metal.Network]          { return ds.network }
func (ds *datastore) FilesystemLayout() interfaces.Storage[*metal.FilesystemLayout] { return ds.fsl }
func (ds *datastore) Image() interfaces.Storage[*metal.Image]              { return ds.image }
func (ds *datastore) Switch() interfaces.Storage[*metal.Switch]            { return ds.sw }
func (ds *datastore) SwitchStatus() interfaces.Storage[*metal.SwitchStatus] { return ds.switchStatus }
func (ds *datastore) Event() interfaces.Storage[*metal.ProvisioningEventContainer] { return ds.event }
func (ds *datastore) AsnPool() interfaces.IntegerPool                      { return ds.asnPool }
func (ds *datastore) VrfPool() interfaces.IntegerPool                      { return ds.vrfPool }
func (ds *datastore) GetTableNames() []string                              { return ds.tableNames }
