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
	Store struct {
		log   *slog.Logger
		ds    generic.Datastore
		mdc   mdm.Client
		ipam  ipamv1connect.IpamServiceClient
		async *asyncclient.Client
	}

	store[S any, E Entity, M Message, C CreateMessage, U UpdateMessage, Q Query] struct {
		s     *Store
		typed S
		repository[E, M, C, U, Q]
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
	return r.ip(&ProjectScope{
		projectID: project,
	})
}

func (r *Store) UnscopedIP() IP {
	return r.ip(nil)
}

func (r *Store) ip(scope *ProjectScope) IP {
	return &store[*ipRepository, *metal.IP, *apiv2.IP, *apiv2.IPServiceCreateRequest, *apiv2.IPServiceUpdateRequest, *apiv2.IPQuery]{
		repository: &ipRepository{
			r:     r,
			scope: scope,
		},
	}
}

func (r *Store) Image() Image {
	repository := &imageRepository{
		r: r,
	}

	return &store[*imageRepository, *metal.Image, *apiv2.Image, *adminv2.ImageServiceCreateRequest, *adminv2.ImageServiceUpdateRequest, *apiv2.ImageQuery]{
		repository: repository,
		s:          r,
		typed:      repository,
	}
}

func (r *Store) Network(project string) Network {
	return r.network(&ProjectScope{
		projectID: project,
	})
}

func (r *Store) UnscopedNetwork() Network {
	return r.network(nil)
}

func (r *Store) network(scope *ProjectScope) Network {
	repository := &networkRepository{
		r:     r,
		scope: scope,
	}

	return &store[*networkRepository, *metal.Network, *apiv2.Network, *apiv2.NetworkServiceCreateRequest, *apiv2.NetworkServiceUpdateRequest, *apiv2.NetworkServiceListRequest]{
		repository: repository,
		s:          r,
		typed:      repository,
	}
}

func (r *Store) Project(project string) Project {
	return r.project(&ProjectScope{
		projectID: project,
	})
}

func (r *Store) UnscopedProject() Project {
	return r.project(nil)
}

func (r *Store) project(scope *ProjectScope) Project {
	repository := &projectRepository{
		r:     r,
		scope: scope,
	}

	return &store[*projectRepository, *mdcv1.Project, *apiv2.Project, *apiv2.ProjectServiceCreateRequest, *apiv2.ProjectServiceUpdateRequest, *apiv2.ProjectServiceListRequest]{
		repository: repository,
		s:          r,
		typed:      repository,
	}
}

func (r *Store) Tenant() Tenant {
	repository := &tenantRepository{
		r: r,
	}

	return &store[*tenantRepository, *mdcv1.Tenant, *apiv2.Tenant, *apiv2.TenantServiceCreateRequest, *apiv2.TenantServiceUpdateRequest, *apiv2.TenantServiceListRequest]{
		repository: repository,
		s:          r,
		typed:      repository,
	}
}

func (r *Store) FilesystemLayout() FilesystemLayout {
	repository := &filesystemLayoutRepository{
		r: r,
	}

	return &store[*filesystemLayoutRepository, *metal.FilesystemLayout, *apiv2.FilesystemLayout, *adminv2.FilesystemServiceCreateRequest, *adminv2.FilesystemServiceUpdateRequest, *apiv2.FilesystemServiceListRequest]{
		repository: repository,
		s:          r,
		typed:      repository,
	}
}
func (r *Store) Partition() Partition {
	repository := &partitionRepository{
		r: r,
	}

	return &store[*partitionRepository, *metal.Partition, *apiv2.Partition, *adminv2.PartitionServiceCreateRequest, *adminv2.PartitionServiceUpdateRequest, *apiv2.PartitionQuery]{
		repository: repository,
		s:          r,
		typed:      repository,
	}
}

func (s *store[R, E, M, C, U, Q]) ConvertToInternal(msg M) (E, error) {
	return s.convertToInternal(msg)
}

func (s *store[R, E, M, C, U, Q]) ConvertToProto(e E) (M, error) {
	return s.convertToProto(e)
}

func (s *store[R, E, M, C, U, Q]) Create(ctx context.Context, c C) (E, error) {
	var zero E

	err := s.validateCreate(ctx, c)
	if err != nil {
		return zero, err
	}

	return s.create(ctx, c)
}

func (s *store[R, E, M, C, U, Q]) Delete(ctx context.Context, id string) (E, error) {
	var zero E

	e, err := s.get(ctx, id)
	if err != nil {
		return zero, err
	}

	err = s.validateDelete(ctx, e)
	if err != nil {
		return zero, err
	}

	err = s.delete(ctx, e)
	if err != nil {
		return zero, err
	}

	return e, nil
}

func (s *store[R, E, M, C, U, Q]) Find(ctx context.Context, query Q) (E, error) {
	return s.find(ctx, query)
}

func (s *store[R, E, M, C, U, Q]) Get(ctx context.Context, id string) (E, error) {
	var zero E

	e, err := s.get(ctx, id)
	if err != nil {
		return zero, err
	}

	// FIXME: this will break the tests!
	// ok := s.matchScope(e)
	// if !ok {
	// 	return zero, errorutil.NotFound("entity %q not found", id)
	// }

	return e, nil
}

func (s *store[R, E, M, C, U, Q]) List(ctx context.Context, query Q) ([]E, error) {
	return s.list(ctx, query)
}

func (s *store[R, E, M, C, U, Q]) Update(ctx context.Context, id string, u U) (E, error) {
	var zero E

	e, err := s.get(ctx, id)
	if err != nil {
		return zero, err
	}

	err = s.validateUpdate(ctx, u, e)
	if err != nil {
		return zero, err
	}

	return s.update(ctx, e, u)
}

func (s *store[R, E, M, C, U, Q]) AdditionalMethods() R {
	return s.typed
}
