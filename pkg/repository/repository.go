package repository

import (
	"context"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
)

type (
	Repository[R Repo, E Entity, M Message, C CreateMessage, U UpdateMessage, Q Query] interface {
		Get(ctx context.Context, id string) (E, error)

		Create(ctx context.Context, c C) (E, error)

		Update(ctx context.Context, id string, u U) (E, error)

		Delete(ctx context.Context, id string) (E, error)

		Find(ctx context.Context, query Q) (E, error)
		List(ctx context.Context, query Q) ([]E, error)

		ConvertToInternal(ctx context.Context, msg M) (E, error)
		ConvertToProto(ctx context.Context, e E) (M, error)

		AdditionalMethods() R
	}

	repository[E Entity, M Message, C CreateMessage, U UpdateMessage, Q Query] interface {
		get(ctx context.Context, id string) (E, error)

		validateCreate(ctx context.Context, create C) error
		create(ctx context.Context, c C) (E, error)

		validateUpdate(ctx context.Context, msg U, old E) error
		update(ctx context.Context, e E, msg U) (E, error)

		validateDelete(ctx context.Context, e E) error
		delete(ctx context.Context, e E) error

		find(ctx context.Context, query Q) (E, error)
		list(ctx context.Context, query Q) ([]E, error)

		convertToInternal(ctx context.Context, msg M) (E, error)
		convertToProto(ctx context.Context, e E) (M, error)

		matchScope(e E) bool
	}

	// Repo is the typed repository in order to expose public functions on the repository to the consumers.
	Repo any
	// Entity is the internal representation of an api resource, which is stored in the backend.
	Entity any
	// Message is the external representation of an api resource for consumers.
	Message any
	// UpdateMessage is an external request to update an entity for consumers.
	UpdateMessage any
	// CreateMessage is an external request to create an entity for consumers.
	// TODO: ideally all update messages should clearly expose the identifier in order to get the entity with it!
	// UpdateMessage interface{ ID() string }
	CreateMessage any
	// Query is an external representation to filter an entity for consumers.
	Query any

	ProjectScope struct {
		projectID string
	}
	TenantScope struct {
		tenantID string
	}

	IP interface {
		Repository[*ipRepository, *metal.IP, *apiv2.IP, *apiv2.IPServiceCreateRequest, *apiv2.IPServiceUpdateRequest, *apiv2.IPQuery]
	}
	Machine interface {
		Repository[*machineRepository, *metal.Machine, *apiv2.Machine, *apiv2.MachineServiceCreateRequest, *apiv2.MachineServiceUpdateRequest, *apiv2.MachineQuery]
	}

	Network interface {
		Repository[*networkRepository, *metal.Network, *apiv2.Network, *adminv2.NetworkServiceCreateRequest, *adminv2.NetworkServiceUpdateRequest, *apiv2.NetworkQuery]
	}

	Project interface {
		Repository[*projectRepository, *mdcv1.Project, *apiv2.Project, *apiv2.ProjectServiceCreateRequest, *apiv2.ProjectServiceUpdateRequest, *apiv2.ProjectServiceListRequest]
	}

	ProjectMember interface {
		Repository[*projectMemberRepository, *mdcv1.ProjectMember, *apiv2.ProjectMember, *ProjectMemberCreateRequest, *ProjectMemberUpdateRequest, *ProjectMemberQuery]
	}

	Tenant interface {
		Repository[*tenantRepository, *mdcv1.Tenant, *apiv2.Tenant, *apiv2.TenantServiceCreateRequest, *apiv2.TenantServiceUpdateRequest, *apiv2.TenantServiceListRequest]
	}

	TenantMember interface {
		Repository[*tenantMemberRepository, *mdcv1.TenantMember, *mdcv1.TenantMember, *TenantMemberCreateRequest, *TenantMemberUpdateRequest, *TenantMemberQuery]
	}

	FilesystemLayout interface {
		Repository[*filesystemLayoutRepository, *metal.FilesystemLayout, *apiv2.FilesystemLayout, *adminv2.FilesystemServiceCreateRequest, *adminv2.FilesystemServiceUpdateRequest, *apiv2.FilesystemServiceListRequest]
	}
	Size interface {
		Repository[*sizeRepository, *metal.Size, *apiv2.Size, *adminv2.SizeServiceCreateRequest, *adminv2.SizeServiceUpdateRequest, *apiv2.SizeQuery]
	}

	Image interface {
		Repository[*imageRepository, *metal.Image, *apiv2.Image, *adminv2.ImageServiceCreateRequest, *adminv2.ImageServiceUpdateRequest, *apiv2.ImageQuery]
	}

	Partition interface {
		Repository[*partitionRepository, *metal.Partition, *apiv2.Partition, *adminv2.PartitionServiceCreateRequest, *adminv2.PartitionServiceUpdateRequest, *apiv2.PartitionQuery]
	}
)
