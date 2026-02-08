package generic

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

const demotedUser = "metal"

type (
	datastore struct {
		log           *slog.Logger
		queryExecutor r.QueryExecutor
		dbname        string

		ip              *storage[*metal.IP]
		machine         *storage[*metal.Machine]
		event           *storage[*metal.ProvisioningEventContainer]
		size            *storage[*metal.Size]
		sizeReservation *storage[*metal.SizeReservation]
		partition       *storage[*metal.Partition]
		network         *storage[*metal.Network]
		fsl             *storage[*metal.FilesystemLayout]
		image           *storage[*metal.Image]
		sw              *storage[*metal.Switch]
		switchStatus    *storage[*metal.SwitchStatus]

		asnPool *integerPool
		vrfPool *integerPool
	}
)

func New(log *slog.Logger, opts r.ConnectOpts, dsOpts ...dataStoreOption) (*datastore, error) {
	// the datastore runs with the metal user (not admin user) that cannot write during migrations
	opts.Username = demotedUser
	log = log.WithGroup("datastore")

	log.Info("create rethinkdb client", "addresses", opts.Addresses, "dbname", opts.Database, "user", opts.Username, "password", opts.Password)

	session, err := r.Connect(opts)
	if err != nil {
		return nil, err
	}

	ds := &datastore{
		log:           log,
		queryExecutor: session,
		dbname:        opts.Database,
	}

	ds.ip = newStorage[*metal.IP](ds, "ip")
	ds.machine = newStorage[*metal.Machine](ds, "machine")
	ds.size = newStorage[*metal.Size](ds, "size")
	ds.sizeReservation = newStorage[*metal.SizeReservation](ds, "sizereservation")
	ds.partition = newStorage[*metal.Partition](ds, "partition")
	ds.network = newStorage[*metal.Network](ds, "network")
	ds.fsl = newStorage[*metal.FilesystemLayout](ds, "filesystemlayout")
	ds.image = newStorage[*metal.Image](ds, "image")
	ds.event = newStorage[*metal.ProvisioningEventContainer](ds, "event")
	ds.sw = newStorage[*metal.Switch](ds, "switch")
	ds.switchStatus = newStorage[*metal.SwitchStatus](ds, "switchstatus")

	var (
		vrfMin = uint(1)
		vrfMax = uint(131072)
		asnMin = uint(1)
		asnMax = uint(131072)
	)

	for _, opt := range dsOpts {
		switch o := opt.(type) {
		case *vrfPoolRange:
			vrfMin = o.min
			vrfMax = o.max
		case *asnPoolRange:
			asnMin = o.min
			asnMax = o.max
		default:
			return nil, fmt.Errorf("unknown datastore opt: %T", opt)
		}
	}

	ds.asnPool = newIntegerPool(ds, asnIntegerPool, "asnpool", asnMin, asnMax)
	ds.vrfPool = newIntegerPool(ds, vrfIntegerPool, "integerpool", vrfMin, vrfMax)

	return ds, nil
}

func (ds *datastore) Version(ctx context.Context) (string, error) {
	cursor, err := r.DB("rethinkdb").Table("server_status").Field("process").Field("version").Run(ds.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		return "", err
	}

	var version string

	err = cursor.One(&version)
	if err != nil {
		return "", err
	}

	return version, nil
}

func (ds *datastore) IP() Storage[*metal.IP] {
	return ds.ip
}

func (ds *datastore) Machine() Storage[*metal.Machine] {
	return ds.machine
}

func (ds *datastore) Size() Storage[*metal.Size] {
	return ds.size
}

func (ds *datastore) SizeReservation() Storage[*metal.SizeReservation] {
	return ds.sizeReservation
}

func (ds *datastore) Partition() Storage[*metal.Partition] {
	return ds.partition
}

func (ds *datastore) Network() Storage[*metal.Network] {
	return ds.network
}

func (ds *datastore) FilesystemLayout() Storage[*metal.FilesystemLayout] {
	return ds.fsl
}

func (ds *datastore) Image() Storage[*metal.Image] {
	return ds.image
}

func (ds *datastore) Switch() Storage[*metal.Switch] {
	return ds.sw
}

func (ds *datastore) SwitchStatus() Storage[*metal.SwitchStatus] {
	return ds.switchStatus
}

func (ds *datastore) Event() Storage[*metal.ProvisioningEventContainer] {
	return ds.event
}

func (ds *datastore) AsnPool() *integerPool {
	return ds.asnPool
}

func (ds *datastore) VrfPool() *integerPool {
	return ds.vrfPool
}
