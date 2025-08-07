package repository

import (
	"context"
	"log/slog"

	asyncclient "github.com/metal-stack/metal-apiserver/pkg/async/client"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"

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

	store[R Repo, E Entity, M Message, C CreateMessage, U UpdateMessage, Q Query] struct {
		typed R
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

func (s *Store) IP(project string) IP {
	return s.ip(&ProjectScope{
		projectID: project,
	})
}

func (s *Store) UnscopedIP() IP {
	return s.ip(nil)
}

func (s *Store) ip(scope *ProjectScope) IP {
	return &store[*ipRepository, *metal.IP, *apiv2.IP, *apiv2.IPServiceCreateRequest, *apiv2.IPServiceUpdateRequest, *apiv2.IPQuery]{
		repository: &ipRepository{
			s:     s,
			scope: scope,
		},
	}
}

func (s *Store) Image() Image {
	repository := &imageRepository{
		s: s,
	}

	return &store[*imageRepository, *metal.Image, *apiv2.Image, *adminv2.ImageServiceCreateRequest, *adminv2.ImageServiceUpdateRequest, *apiv2.ImageQuery]{
		repository: repository,
		typed:      repository,
	}
}

func (s *Store) Network(project string) Network {
	return s.network(&ProjectScope{
		projectID: project,
	})
}

func (s *Store) UnscopedNetwork() Network {
	return s.network(nil)
}

func (s *Store) network(scope *ProjectScope) Network {
	repository := &networkRepository{
		s:     s,
		scope: scope,
	}

	return &store[*networkRepository, *metal.Network, *apiv2.Network, *adminv2.NetworkServiceCreateRequest, *adminv2.NetworkServiceUpdateRequest, *apiv2.NetworkQuery]{
		repository: repository,
		typed:      repository,
	}
}

func (s *Store) Project(project string) Project {
	return s.project(&ProjectScope{
		projectID: project,
	})
}

func (s *Store) UnscopedProject() Project {
	return s.project(nil)
}

func (s *Store) project(scope *ProjectScope) Project {
	repository := &projectRepository{
		s:     s,
		scope: scope,
	}

	return &store[*projectRepository, *mdcv1.Project, *apiv2.Project, *apiv2.ProjectServiceCreateRequest, *apiv2.ProjectServiceUpdateRequest, *apiv2.ProjectServiceListRequest]{
		repository: repository,
		typed:      repository,
	}
}

func (s *Store) Tenant() Tenant {
	repository := &tenantRepository{
		s: s,
	}

	return &store[*tenantRepository, *mdcv1.Tenant, *apiv2.Tenant, *apiv2.TenantServiceCreateRequest, *apiv2.TenantServiceUpdateRequest, *apiv2.TenantServiceListRequest]{
		repository: repository,
		typed:      repository,
	}
}

func (s *Store) FilesystemLayout() FilesystemLayout {
	repository := &filesystemLayoutRepository{
		s: s,
	}

	return &store[*filesystemLayoutRepository, *metal.FilesystemLayout, *apiv2.FilesystemLayout, *adminv2.FilesystemServiceCreateRequest, *adminv2.FilesystemServiceUpdateRequest, *apiv2.FilesystemServiceListRequest]{
		repository: repository,
		typed:      repository,
	}
}

func (s *Store) Size() Size {
	repository := &sizeRepository{
		s: s,
	}

	return &store[*sizeRepository, *metal.Size, *apiv2.Size, *adminv2.SizeServiceCreateRequest, *adminv2.SizeServiceUpdateRequest, *apiv2.SizeQuery]{
		repository: repository,
		typed:      repository,
	}
}

func (s *Store) Partition() Partition {
	repository := &partitionRepository{
		s: s,
	}

	return &store[*partitionRepository, *metal.Partition, *apiv2.Partition, *adminv2.PartitionServiceCreateRequest, *adminv2.PartitionServiceUpdateRequest, *apiv2.PartitionQuery]{
		repository: repository,
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

	ok := s.matchScope(e)
	if !ok {
		return zero, errorutil.NotFound("%T with id %q not found", e, id)
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

	ok := s.matchScope(e)
	if !ok {
		return zero, errorutil.NotFound("%T with id %q not found", e, id)
	}

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

	ok := s.matchScope(e)
	if !ok {
		return zero, errorutil.NotFound("%T with id %q not found", e, id)
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
