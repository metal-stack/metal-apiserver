package generic

import (
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

		ip        *storage[*metal.IP]
		partition *storage[*metal.Partition]
		network   *storage[*metal.Network]
		fsl       *storage[*metal.FilesystemLayout]
		image     *storage[*metal.Image]
	}
)

func New(log *slog.Logger, opts r.ConnectOpts) (*datastore, error) {
	// the datastore runs with the metal user (not admin user) that cannot write during migrations
	opts.Username = demotedUser

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
	ds.partition = newStorage[*metal.Partition](ds, "partition")
	ds.network = newStorage[*metal.Network](ds, "network")
	ds.fsl = newStorage[*metal.FilesystemLayout](ds, "filesystemlayout")
	ds.image = newStorage[*metal.Image](ds, "image")

	return ds, nil
}

func (ds *datastore) IP() Storage[*metal.IP] {
	return ds.ip
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
