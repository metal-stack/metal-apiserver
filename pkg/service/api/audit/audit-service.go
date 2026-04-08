package audit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
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
	disabled bool
	c        auditing.Auditing
	repo     *repository.Store
}

func New(c Config) apiv2connect.AuditServiceHandler {
	return &auditServiceServer{
		log:      c.Log.WithGroup("auditService"),
		disabled: c.AuditClient == nil,
		c:        c.AuditClient,
		repo:     c.Repo,
	}
}

func (a *auditServiceServer) Get(ctx context.Context, rq *apiv2.AuditServiceGetRequest) (*apiv2.AuditServiceGetResponse, error) {
	if a.disabled {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("the audit backend is currently disabled"))
	}

	phase := apiv2.AuditPhase_AUDIT_PHASE_REQUEST
	if rq.Phase != nil {
		phase = *rq.Phase
	}

	traces, err := a.repo.Audit(rq.Login).List(ctx, &apiv2.AuditQuery{
		Uuid:  &rq.Uuid,
		Phase: &phase,
	})
	if err != nil {
		return nil, err
	}

	switch len(traces) {
	case 0:
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no audit trace found for given request id %q", rq.Uuid))
	case 1:
		return &apiv2.AuditServiceGetResponse{Trace: traces[0]}, nil
	default:
		return nil, connect.NewError(connect.CodeInternal, errors.New("unable to find distinct audit entry, search result is ambiguous"))
	}
}

func (a *auditServiceServer) List(ctx context.Context, rq *apiv2.AuditServiceListRequest) (*apiv2.AuditServiceListResponse, error) {
	if a.disabled {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("the audit backend is currently disabled"))
	}

	traces, err := a.repo.Audit(rq.Login).List(ctx, rq.Query)
	if err != nil {
		return nil, err
	}

	return &apiv2.AuditServiceListResponse{Traces: traces}, nil
}
