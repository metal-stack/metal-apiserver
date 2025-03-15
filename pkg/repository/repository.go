package repository

import (
	"context"
	"log/slog"

	asyncclient "github.com/metal-stack/metal-apiserver/pkg/async/client"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/redis/go-redis/v9"
)

type (
	Validated[M any] struct {
		message M
	}
	Repository[E Entity, M Message, C CreateMessage, U UpdateMessage, Q Query] interface {
		Get(ctx context.Context, id string) (E, error)

		ValidateCreate(ctx context.Context, create C) (*Validated[C], error)
		Create(ctx context.Context, c *Validated[C]) (E, error)

		ValidateUpdate(ctx context.Context, msg U) (*Validated[U], error)
		Update(ctx context.Context, msg *Validated[U]) (E, error)

		ValidateDelete(ctx context.Context, e E) (*Validated[E], error)
		Delete(ctx context.Context, e *Validated[E]) (E, error)

		Find(ctx context.Context, query Q) (E, error)
		List(ctx context.Context, query Q) ([]E, error)

		ConvertToInternal(msg M) (E, error)
		ConvertToProto(e E) (M, error)

		MatchScope(e E) error
	}

	Entity        any
	Message       any
	UpdateMessage any
	CreateMessage any
	Query         any

	Store struct {
		log   *slog.Logger
		ds    generic.Datastore
		mdc   mdm.Client
		ipam  ipamv1connect.IpamServiceClient
		async *asyncclient.Client
	}

	ProjectScope struct {
		projectID string
	}

	IP interface {
		Repository[*metal.IP, *apiv2.IP, *apiv2.IPServiceCreateRequest, *apiv2.IPServiceUpdateRequest, *apiv2.IPQuery]
		// TODO define additional methods only for the IP repository
		// AdditionalMethod()
	}

	Network interface {
		Repository[*metal.Network, *apiv2.Network, *apiv2.NetworkServiceCreateRequest, *apiv2.NetworkServiceUpdateRequest, *apiv2.NetworkServiceListRequest]
	}

	Project interface {
		Repository[*mdcv1.Project, *apiv2.Project, *apiv2.ProjectServiceCreateRequest, *apiv2.ProjectServiceUpdateRequest, *apiv2.ProjectServiceListRequest]
	}

	FilesystemLayout interface {
		Repository[*metal.FilesystemLayout, *apiv2.FilesystemLayout, *adminv2.FilesystemServiceCreateRequest, *adminv2.FilesystemServiceUpdateRequest, *apiv2.FilesystemServiceListRequest]
	}

	Image interface {
		Repository[*metal.Image, *apiv2.Image, *adminv2.ImageServiceCreateRequest, *adminv2.ImageServiceUpdateRequest, *apiv2.ImageQuery]
		GetMostRecentImageFor(id string, images []*metal.Image) (*metal.Image, error)
		SortImages(images []*metal.Image) []*metal.Image
	}
)

func New(log *slog.Logger, mdc mdm.Client, ds generic.Datastore, ipam ipamv1connect.IpamServiceClient, redis *redis.Client) (*Store, error) {
	r := &Store{
		log:   log,
		mdc:   mdc,
		ipam:  ipam,
		ds:    ds,
		async: asyncclient.New(log, redis),
	}

	return r, nil
}

func (r *Store) IP(project string) IP {
	return &ipRepository{
		r: r,
		scope: &ProjectScope{
			projectID: project,
		},
	}
}

func (r *Store) UnscopedIP() IP {
	return &ipRepository{
		r:     r,
		scope: nil,
	}
}

func (r *Store) Image() Image {
	return &imageRepository{
		r: r,
	}
}

func (r *Store) Network(project *string) Network {
	var scope *ProjectScope
	if project != nil {
		scope = &ProjectScope{
			projectID: *project,
		}
	}
	return &networkRepository{
		r:     r,
		scope: scope,
	}
}

func (r *Store) Project(project *string) Project {
	var scope *ProjectScope
	if project != nil {
		scope = &ProjectScope{
			projectID: *project,
		}
	}
	return &projectRepository{
		r:     r,
		scope: scope,
	}
}
func (r *Store) FilesystemLayout() FilesystemLayout {
	return &filesystemLayoutRepository{
		r: r,
	}
}
