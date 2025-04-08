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
	ValidatedUpdate[E Entity, M any] struct {
		message M
		// entity is the fetched entity at the point of the validation.
		// it can be modified for the update and used for applying the update in the update function.
		entity E
	}
	Repository[E Entity, M Message, C CreateMessage, U UpdateMessage, Q Query] interface {
		Get(ctx context.Context, id string) (E, error)

		ValidateCreate(ctx context.Context, create C) (*Validated[C], error)
		Create(ctx context.Context, c *Validated[C]) (E, error)

		// ValidateUpdate(ctx context.Context, msg U) (*ValidatedUpdate[E, U], error)
		Update(ctx context.Context, msg *ValidatedUpdate[E, U]) (E, error)

		ValidateDelete(ctx context.Context, e E) (*Validated[E], error)
		Delete(ctx context.Context, e *Validated[E]) (E, error)

		Find(ctx context.Context, query Q) (E, error)
		List(ctx context.Context, query Q) ([]E, error)

		ConvertToInternal(msg M) (E, error)
		ConvertToProto(e E) (M, error)

		MatchScope(e E) error
	}

	repository[E Entity, M Message, C CreateMessage, U UpdateMessage, Q Query] interface {
		get(ctx context.Context, id string) (E, error)

		validateCreate(ctx context.Context, create C) (*Validated[C], error)
		create(ctx context.Context, c *Validated[C]) (E, error)

		validateUpdate(ctx context.Context, msg U, old E) (*ValidatedUpdate[E, U], error)
		update(ctx context.Context, msg *ValidatedUpdate[E, U]) (E, error)

		validateDelete(ctx context.Context, e E) (*Validated[E], error)
		delete(ctx context.Context, e *Validated[E]) (E, error)

		find(ctx context.Context, query Q) (E, error)
		list(ctx context.Context, query Q) ([]E, error)

		convertToInternal(msg M) (E, error)
		convertToProto(e E) (M, error)

		matchScope(e E) error
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

	store[E Entity, M Message, C CreateMessage, U UpdateMessage, Q Query] struct {
		impl repository[E, M, C, U, Q]
	}

	ProjectScope struct {
		projectID string
	}

	IP interface {
		Repository[*metal.IP, *apiv2.IP, *apiv2.IPServiceCreateRequest, *apiv2.IPServiceUpdateRequest, *apiv2.IPQuery]
	}

	Network interface {
		Repository[*metal.Network, *apiv2.Network, *apiv2.NetworkServiceCreateRequest, *apiv2.NetworkServiceUpdateRequest, *apiv2.NetworkServiceListRequest]
	}

	Project interface {
		Repository[*mdcv1.Project, *apiv2.Project, *apiv2.ProjectServiceCreateRequest, *apiv2.ProjectServiceUpdateRequest, *apiv2.ProjectServiceListRequest]
	}
	Tenant interface {
		Repository[*mdcv1.Tenant, *apiv2.Tenant, *apiv2.TenantServiceCreateRequest, *apiv2.TenantServiceUpdateRequest, *apiv2.TenantServiceListRequest]
	}

	FilesystemLayout interface {
		Repository[*metal.FilesystemLayout, *apiv2.FilesystemLayout, *adminv2.FilesystemServiceCreateRequest, *adminv2.FilesystemServiceUpdateRequest, *apiv2.FilesystemServiceListRequest]
	}

	Image interface {
		Repository[*metal.Image, *apiv2.Image, *adminv2.ImageServiceCreateRequest, *adminv2.ImageServiceUpdateRequest, *apiv2.ImageQuery]
		GetMostRecentImageFor(id string, images []*metal.Image) (*metal.Image, error)
		SortImages(images []*metal.Image) []*metal.Image
	}

	Partition interface {
		Repository[*metal.Partition, *apiv2.Partition, *adminv2.PartitionServiceCreateRequest, *adminv2.PartitionServiceUpdateRequest, *apiv2.PartitionQuery]
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
	return &store[*metal.IP, *apiv2.IP, *apiv2.IPServiceCreateRequest, *apiv2.IPServiceUpdateRequest, *apiv2.IPQuery]{
		impl: &ipRepository{
			r: r,
			scope: &ProjectScope{
				projectID: project,
			},
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

func (r *Store) Network(project string) Network {
	return &networkRepository{
		r: r,
		scope: &ProjectScope{
			projectID: project,
		},
	}
}

func (r *Store) UnscopedNetwork() Network {
	return &networkRepository{
		r:     r,
		scope: nil,
	}
}
func (r *Store) Project(project string) Project {
	return &projectRepository{
		r: r,
		scope: &ProjectScope{
			projectID: project,
		},
	}
}

func (r *Store) UnscopedProject() Project {
	return &projectRepository{
		r:     r,
		scope: nil,
	}
}

func (r *Store) Tenant() Tenant {
	return &tenantRepository{
		r: r,
	}
}

func (r *Store) FilesystemLayout() FilesystemLayout {
	return &filesystemLayoutRepository{
		r: r,
	}
}
func (r *Store) Partition() Partition {
	return &partitionRepository{
		r: r,
	}
}

// ConvertToInternal implements IP.
func (s *store[E, M, C, U, Q]) ConvertToInternal(msg *apiv2.IP) (*metal.IP, error) {
	return s.impl.convertToInternal(msg)
}

// ConvertToProto implements IP.
func (s *store[E, M, C, U, Q]) ConvertToProto(e *metal.IP) (*apiv2.IP, error) {
	panic("unimplemented")
}

// Create implements IP.
func (s *store[E, M, C, U, Q]) Create(ctx context.Context, c *Validated[*apiv2.IPServiceCreateRequest]) (*metal.IP, error) {
	panic("unimplemented")
}

// Delete implements IP.
func (s *store[E, M, C, U, Q]) Delete(ctx context.Context, e *Validated[*metal.IP]) (*metal.IP, error) {
	panic("unimplemented")
}

// Find implements IP.
func (s *store[E, M, C, U, Q]) Find(ctx context.Context, query *apiv2.IPQuery) (*metal.IP, error) {
	panic("unimplemented")
}

// Get implements IP.
func (s *store[E, M, C, U, Q]) Get(ctx context.Context, id string) (*metal.IP, error) {
	panic("unimplemented")
}

// List implements IP.
func (s *store[E, M, C, U, Q]) List(ctx context.Context, query *apiv2.IPQuery) ([]*metal.IP, error) {
	panic("unimplemented")
}

// MatchScope implements IP.
func (s *store[E, M, C, U, Q]) MatchScope(e *metal.IP) error {
	panic("unimplemented")
}

// Update implements IP.
func (s *store[E, M, C, U, Q]) Update(ctx context.Context, msg *ValidatedUpdate[*metal.IP, *apiv2.IPServiceUpdateRequest]) (*metal.IP, error) {
	panic("unimplemented")
}

// ValidateCreate implements IP.
func (s *store[E, M, C, U, Q]) ValidateCreate(ctx context.Context, create *apiv2.IPServiceCreateRequest) (*Validated[*apiv2.IPServiceCreateRequest], error) {
	panic("unimplemented")
}

// ValidateDelete implements IP.
func (s *store[E, M, C, U, Q]) ValidateDelete(ctx context.Context, e *metal.IP) (*Validated[*metal.IP], error) {
	panic("unimplemented")
}

// ValidateUpdate implements IP.
func (s *store[E, M, C, U, Q]) ValidateUpdate(ctx context.Context, msg *apiv2.IPServiceUpdateRequest) (*ValidatedUpdate[*metal.IP, *apiv2.IPServiceUpdateRequest], error) {
	panic("unimplemented")
}
