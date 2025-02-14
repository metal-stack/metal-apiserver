package repository

import (
	"log/slog"

	"github.com/metal-stack/api-server/pkg/db/generic"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"
)

type Repository struct {
	log  *slog.Logger
	ds   *generic.Datastore
	mdc  mdm.Client
	ipam ipamv1connect.IpamServiceClient
}

type ProjectScope string

func New(log *slog.Logger, mdc mdm.Client, ds *generic.Datastore, ipam ipamv1connect.IpamServiceClient) *Repository {
	return &Repository{
		log:  log,
		mdc:  mdc,
		ipam: ipam,
		ds:   ds,
	}
}

func (r *Repository) IP(scope ProjectScope) *ipRepository {
	return &ipRepository{
		r:     r,
		scope: scope,
	}
}

func (r *Repository) UnscopedIP() *ipUnscopedRepository {
	return &ipUnscopedRepository{
		r: r,
	}
}

func (r *Repository) Network(scope ProjectScope) *networkRepository {
	return &networkRepository{
		r:     r,
		scope: scope,
	}
}

func (r *Repository) Project() *projectRepository {
	return &projectRepository{
		r: r,
	}
}
func (r *Repository) FilesystemLayout() *filesystemRepository {
	return &filesystemRepository{
		r: r,
	}
}
