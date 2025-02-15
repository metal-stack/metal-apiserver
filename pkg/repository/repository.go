package repository

import (
	"context"
	"log/slog"

	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/metal"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"
)

type (
	Repository[E Entity, M Message, C CreateMessage, U UpdateMessage, Q Query] interface {
		Get(ctx context.Context, id string) (E, error)
		Create(ctx context.Context, c C) (E, error)
		Update(ctx context.Context, msg U) (E, error)
		Delete(ctx context.Context, e E) (E, error)
		Find(ctx context.Context, query Q) (E, error)
		List(ctx context.Context, query Q) ([]E, error)
		ConvertToInternal(msg M) (E, error)
		ConvertToProto(e E) (M, error)
	}

	Entity        any
	Message       any
	UpdateMessage any
	CreateMessage any
	Query         any

	Repostore struct { // TODO naming
		log  *slog.Logger
		ds   *generic.Datastore
		mdc  mdm.Client
		ipam ipamv1connect.IpamServiceClient
	}

	ProjectScope string
)

func New(log *slog.Logger, mdc mdm.Client, ds *generic.Datastore, ipam ipamv1connect.IpamServiceClient) *Repostore {
	return &Repostore{
		log:  log,
		mdc:  mdc,
		ipam: ipam,
		ds:   ds,
	}
}

func (r *Repostore) IP(scope ProjectScope) Repository[*metal.IP, *apiv2.IP, *apiv2.IPServiceCreateRequest, *apiv2.IPServiceUpdateRequest, *apiv2.IPServiceListRequest] {
	return &ipRepository{
		r:     r,
		scope: scope,
	}
}

func (r *Repostore) UnscopedIP() Repository[*metal.IP, *apiv2.IP, *apiv2.IPServiceCreateRequest, *apiv2.IPServiceUpdateRequest, *apiv2.IPServiceListRequest] {
	return &ipUnscopedRepository{
		r: r,
	}
}

func (r *Repostore) Network(scope ProjectScope) Repository[*metal.Network, *apiv2.Network, *apiv2.NetworkServiceCreateRequest, *apiv2.NetworkServiceUpdateRequest, *apiv2.NetworkServiceListRequest] { // FIXME apiv2 types
	return &networkRepository{
		r:     r,
		scope: scope,
	}
}

func (r *Repostore) Project() Repository[*mdcv1.Project, *apiv2.Project, *apiv2.ProjectServiceCreateRequest, *apiv2.ProjectServiceUpdateRequest, *apiv2.ProjectServiceListRequest] {
	return &projectRepository{
		r: r,
	}
}
func (r *Repostore) FilesystemLayout() Repository[*metal.FilesystemLayout, *apiv2.FilesystemLayout, *adminv2.FilesystemServiceCreateRequest, *adminv2.FilesystemServiceUpdateRequest, *apiv2.FilesystemServiceListRequest] {
	return &filesystemRepository{
		r: r,
	}
}
