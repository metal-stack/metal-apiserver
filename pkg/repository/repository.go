package repository

import (
	"context"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
)

type (
	validated[M any] struct {
		entity M
	}

	validatedDelete[E any] struct {
		entity E
	}
	validatedUpdate[E Entity, M any] struct {
		message M
		// entity is the fetched entity at the point of the validation.
		// it can be modified for the update and used for applying the update in the update function.
		entity E
	}

	Repository[S any, E Entity, M Message, C CreateMessage, U UpdateMessage, Q Query] interface {
		Get(ctx context.Context, id string) (E, error)

		Create(ctx context.Context, c C) (E, error)

		Update(ctx context.Context, id string, u U) (E, error)

		Delete(ctx context.Context, id string) (E, error)

		Find(ctx context.Context, query Q) (E, error)
		List(ctx context.Context, query Q) ([]E, error)

		ConvertToInternal(msg M) (E, error)
		ConvertToProto(e E) (M, error)

		AdditionalMethods() S
	}

	repository[E Entity, M Message, C CreateMessage, U UpdateMessage, Q Query] interface {
		get(ctx context.Context, id string) (E, error)

		validateCreate(ctx context.Context, create C) (*validated[C], error)
		create(ctx context.Context, c *validated[C]) (E, error)

		validateUpdate(ctx context.Context, msg U, old E) (*validatedUpdate[E, U], error)
		update(ctx context.Context, msg *validatedUpdate[E, U]) (E, error)

		validateDelete(ctx context.Context, e E) (*validatedDelete[E], error)
		delete(ctx context.Context, e *validatedDelete[E]) (E, error)

		find(ctx context.Context, query Q) (E, error)
		list(ctx context.Context, query Q) ([]E, error)

		convertToInternal(msg M) (E, error)
		convertToProto(e E) (M, error)

		matchScope(e E) error
	}

	Entity        any
	Message       any
	UpdateMessage any
	// TODO: ideally all update messages should clearly expose the identifier in order to get the entity with it!
	// UpdateMessage interface{ ID() string }
	CreateMessage any
	Query         any

	ProjectScope struct {
		projectID string
	}

	IP interface {
		Repository[*ipRepository, *metal.IP, *apiv2.IP, *apiv2.IPServiceCreateRequest, *apiv2.IPServiceUpdateRequest, *apiv2.IPQuery]
	}

	Network interface {
		Repository[*networkRepository, *metal.Network, *apiv2.Network, *apiv2.NetworkServiceCreateRequest, *apiv2.NetworkServiceUpdateRequest, *apiv2.NetworkServiceListRequest]
	}

	Project interface {
		Repository[*projectRepository, *mdcv1.Project, *apiv2.Project, *apiv2.ProjectServiceCreateRequest, *apiv2.ProjectServiceUpdateRequest, *apiv2.ProjectServiceListRequest]
	}
	Tenant interface {
		Repository[*tenantRepository, *mdcv1.Tenant, *apiv2.Tenant, *apiv2.TenantServiceCreateRequest, *apiv2.TenantServiceUpdateRequest, *apiv2.TenantServiceListRequest]
	}

	FilesystemLayout interface {
		Repository[*filesystemLayoutRepository, *metal.FilesystemLayout, *apiv2.FilesystemLayout, *adminv2.FilesystemServiceCreateRequest, *adminv2.FilesystemServiceUpdateRequest, *apiv2.FilesystemServiceListRequest]
	}

	Image interface {
		Repository[*imageRepository, *metal.Image, *apiv2.Image, *adminv2.ImageServiceCreateRequest, *adminv2.ImageServiceUpdateRequest, *apiv2.ImageQuery]
	}

	Partition interface {
		Repository[*partitionRepository, *metal.Partition, *apiv2.Partition, *adminv2.PartitionServiceCreateRequest, *adminv2.PartitionServiceUpdateRequest, *apiv2.PartitionQuery]
	}
)
