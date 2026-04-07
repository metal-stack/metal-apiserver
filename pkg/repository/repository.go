package repository

import (
	"context"
	"time"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
)

type (
	// Repository implements the basic CRUD operations (business logic) for the consuming API services.
	// It shadows internal data representations from external API representations.
	// It offers scopes during initialization so that resource operations can be scoped by projects and tenants.
	// It ensures that all functions return a connect error, except those called from AdditionalMethods().
	// Therefore, consumers do not need to convert errors to connect errors.
	Repository[R Repo, M Message, C CreateMessage, U UpdateMessage, Q Query] interface {
		// Get returns the API entity with the given id.
		Get(ctx context.Context, id string) (M, error)
		// Create creates the entity from the given create request and returns the API entity.
		Create(ctx context.Context, c C) (M, error)
		// Update updates the entity from the given update request and returns the API entity.
		Update(ctx context.Context, id string, u U) (M, error)
		// Delete deletes the API entity with the given id.
		Delete(ctx context.Context, id string) (M, error)
		// Find returns exactly the one API entity matched by the given query.
		// For multiple or no results an error is returned.
		Find(ctx context.Context, query Q) (M, error)
		// List returns the API entities matched by the given query.
		List(ctx context.Context, query Q) ([]M, error)
		// AdditionalMethods allows access to more specific, non-crud operations of a repository store.
		AdditionalMethods() R
	}

	repository[E Entity, M Message, C CreateMessage, U UpdateMessage, Q Query] interface {
		get(ctx context.Context, id string) (E, error)

		// validateCreate validates the creation request for the entity.
		// every error returned will be wrapped into an InvalidArgument connect error except another connect error is returned.
		validateCreate(ctx context.Context, create C) error
		create(ctx context.Context, c C) (E, error)

		// validateUpdate validates the update of the passed entity.
		// the passed entity was retrieved from the backend so it does not need to be checked if it exists or not.
		// every error returned will be wrapped into an InvalidArgument connect error except another connect error is returned.
		validateUpdate(ctx context.Context, msg U, old E) error
		update(ctx context.Context, e E, msg U) (E, error)

		// validateDelete validates the deletion of the passed entity.
		// the passed entity was retrieved from the backend so it does not need to be checked if it exists or not.
		// every error returned will be wrapped into an InvalidArgument connect error except another connect error is returned.
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
	Entity interface {
		SetChanged(t time.Time)
	}
	// Message is the external representation of an api resource for consumers.
	Message any
	// UpdateMessage is an external request to update an entity for consumers.
	UpdateMessage interface {
		GetUpdateMeta() *apiv2.UpdateMeta
	}
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
		Repository[*ipRepository, *apiv2.IP, *apiv2.IPServiceCreateRequest, *apiv2.IPServiceUpdateRequest, *apiv2.IPQuery]
	}

	Machine interface {
		Repository[*machineRepository, *apiv2.Machine, *apiv2.MachineServiceCreateRequest, *apiv2.MachineServiceUpdateRequest, *apiv2.MachineQuery]
	}

	Network interface {
		Repository[*networkRepository, *apiv2.Network, *adminv2.NetworkServiceCreateRequest, *adminv2.NetworkServiceUpdateRequest, *apiv2.NetworkQuery]
	}

	Project interface {
		Repository[*projectRepository, *apiv2.Project, *apiv2.ProjectServiceCreateRequest, *apiv2.ProjectServiceUpdateRequest, *apiv2.ProjectServiceListRequest]
	}

	ProjectMember interface {
		Repository[*projectMemberRepository, *apiv2.ProjectMember, *api.ProjectMemberCreateRequest, *api.ProjectMemberUpdateRequest, *api.ProjectMemberQuery]
	}

	Tenant interface {
		Repository[*tenantRepository, *apiv2.Tenant, *apiv2.TenantServiceCreateRequest, *apiv2.TenantServiceUpdateRequest, *apiv2.TenantServiceListRequest]
	}

	TenantMember interface {
		Repository[*tenantMemberRepository, *apiv2.TenantMember, *api.TenantMemberCreateRequest, *api.TenantMemberUpdateRequest, *api.TenantMemberQuery]
	}

	FilesystemLayout interface {
		Repository[*filesystemLayoutRepository, *apiv2.FilesystemLayout, *adminv2.FilesystemServiceCreateRequest, *adminv2.FilesystemServiceUpdateRequest, *apiv2.FilesystemServiceListRequest]
	}

	Size interface {
		Repository[*sizeRepository, *apiv2.Size, *adminv2.SizeServiceCreateRequest, *adminv2.SizeServiceUpdateRequest, *apiv2.SizeQuery]
	}

	SizeReservation interface {
		Repository[*sizeReservationRepository, *apiv2.SizeReservation, *adminv2.SizeReservationServiceCreateRequest, *adminv2.SizeReservationServiceUpdateRequest, *apiv2.SizeReservationQuery]
	}

	SizeImageConstraint interface {
		Repository[*sizeImageConstraintRepository, *apiv2.SizeImageConstraint, *adminv2.SizeImageConstraintServiceCreateRequest, *adminv2.SizeImageConstraintServiceUpdateRequest, *apiv2.SizeImageConstraintQuery]
	}

	Image interface {
		Repository[*imageRepository, *apiv2.Image, *adminv2.ImageServiceCreateRequest, *adminv2.ImageServiceUpdateRequest, *apiv2.ImageQuery]
	}

	Partition interface {
		Repository[*partitionRepository, *apiv2.Partition, *adminv2.PartitionServiceCreateRequest, *adminv2.PartitionServiceUpdateRequest, *apiv2.PartitionQuery]
	}

	Switch interface {
		Repository[*switchRepository, *apiv2.Switch, *api.SwitchServiceCreateRequest, *adminv2.SwitchServiceUpdateRequest, *apiv2.SwitchQuery]
	}

	Component interface {
		Repository[*componentRepository, *apiv2.Component, *api.ComponentServiceCreateRequest, *api.ComponentServiceUpdateRequest, *apiv2.ComponentQuery]
	}
	Audit interface {
		Repository[*auditRepository, *apiv2.AuditTrace, any, *auditEntity, *apiv2.AuditQuery]
	}
)
