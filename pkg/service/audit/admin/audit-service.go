package adminaudit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-lib/auditing"
)

type Config struct {
	Log         *slog.Logger
	AuditClient auditing.Auditing
	Repo        *repository.Store
}

type auditServiceServer struct {
	log      *slog.Logger
	repo     repository.Audit
	disabled bool
}

func New(c Config) adminv2connect.AuditServiceHandler {
	return &auditServiceServer{
		log:      c.Log.WithGroup("adminAuditService"),
		disabled: c.AuditClient == nil,
		repo:     c.Repo.UnscopedAudit(c.AuditClient),
	}
}

func (a *auditServiceServer) Get(ctx context.Context, rq *adminv2.AuditServiceGetRequest) (*adminv2.AuditServiceGetResponse, error) {
	if a.disabled {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("the audit backend is currently disabled"))
	}

	phase := apiv2.AuditPhase_AUDIT_PHASE_REQUEST
	if rq.Phase != nil {
		phase = *rq.Phase
	}

	traces, err := a.repo.List(ctx, &apiv2.AuditQuery{
		Uuid:  &rq.Uuid,
		Phase: &phase,
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	switch len(traces) {
	case 0:
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no audit trace found for given request id %q", rq.Uuid))
	case 1:
		return &adminv2.AuditServiceGetResponse{Trace: traces[0]}, nil
	default:
		return nil, connect.NewError(connect.CodeInternal, errors.New("unable to find distinct audit entry, search result is ambiguous"))
	}
}

func (a *auditServiceServer) List(ctx context.Context, rq *adminv2.AuditServiceListRequest) (*adminv2.AuditServiceListResponse, error) {
	if a.disabled {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("the audit backend is currently disabled"))
	}

	traces, err := a.repo.List(ctx, rq.Query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.AuditServiceListResponse{Traces: traces}, nil
}
