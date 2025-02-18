package repository

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/metal"
	"github.com/metal-stack/api-server/pkg/db/tx"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	mdcv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/redis/go-redis/v9"
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
		MatchScope(e E) error
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
		q    *tx.Queue
	}

	ProjectScope struct {
		projectID string
	}
)

func New(log *slog.Logger, mdc mdm.Client, ds *generic.Datastore, ipam ipamv1connect.IpamServiceClient, redis *redis.Client) (*Repostore, error) {

	r := &Repostore{
		log:  log,
		mdc:  mdc,
		ipam: ipam,
		ds:   ds,
	}

	actionFn := r.getActionFn()

	q, err := tx.New(log, redis, actionFn)
	if err != nil {
		return nil, err
	}

	r.q = q

	return r, nil
}

func (r *Repostore) IP(project *string) Repository[*metal.IP, *apiv2.IP, *apiv2.IPServiceCreateRequest, *apiv2.IPServiceUpdateRequest, *apiv2.IPServiceListRequest] {
	var scope *ProjectScope
	if project != nil {
		scope = &ProjectScope{
			projectID: *project,
		}
	}
	return &ipRepository{
		r:     r,
		scope: scope,
	}
}

func (r *Repostore) Network(project *string) Repository[*metal.Network, *apiv2.Network, *apiv2.NetworkServiceCreateRequest, *apiv2.NetworkServiceUpdateRequest, *apiv2.NetworkServiceListRequest] { // FIXME apiv2 types
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

func (r *Repostore) Project(project *string) Repository[*mdcv1.Project, *apiv2.Project, *apiv2.ProjectServiceCreateRequest, *apiv2.ProjectServiceUpdateRequest, *apiv2.ProjectServiceListRequest] {
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
func (r *Repostore) FilesystemLayout() Repository[*metal.FilesystemLayout, *apiv2.FilesystemLayout, *adminv2.FilesystemServiceCreateRequest, *adminv2.FilesystemServiceUpdateRequest, *apiv2.FilesystemServiceListRequest] {
	return &filesystemRepository{
		r: r,
	}
}

func (r *Repostore) getActionFn() tx.ActionFn {
	return func(ctx context.Context, job tx.Job) error {
		if job.ID == "" {
			return fmt.Errorf("job id must not be empty")
		}
		if job.Action == "" {
			return fmt.Errorf("job action must not be empty")
		}
		switch job.Action {
		case tx.ActionIpDelete:
			return r.IpDeleteAction(ctx, job)
		case tx.ActionNetworkDelete:
			return fmt.Errorf("action:%s is not implemented yet", job.Action)
		default:
			return fmt.Errorf("action:%s is not implemented yet", job.Action)
		}
	}
}
