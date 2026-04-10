package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"connectrpc.com/connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-lib/auditing"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	auditingListLimit       = int64(200)
	auditingTimeWindowDelta = 8 * time.Hour
)

type (
	auditEntity struct {
		auditing.Entry
	}

	auditRepository struct {
		c     auditing.Auditing
		scope *TenantScope
	}
)

func (a *auditRepository) list(ctx context.Context, query *apiv2.AuditQuery) ([]*auditEntity, error) {
	var (
		limit = auditingListLimit
		to    = time.Now()
		from  = to.Add(-auditingTimeWindowDelta)
	)

	if query == nil {
		query = &apiv2.AuditQuery{}
	}

	if query.To != nil && !query.To.AsTime().IsZero() {
		to = query.To.AsTime()
	}
	if query.From != nil && !query.From.AsTime().IsZero() {
		from = query.From.AsTime()
	}
	if query.Limit != nil {
		limit = int64(*query.Limit)
	}

	if from.After(to) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid time window, from must be before to"))
	}

	var code *int
	if query.ResultCode != nil {
		code = new(int(*query.ResultCode))
	}

	filter := auditing.EntryFilter{
		Body:       pointer.SafeDeref(query.Body),
		Component:  api.AuditingComponent,
		From:       from,
		Limit:      limit,
		Path:       pointer.SafeDeref(query.Method),
		Phase:      convertToInternalPhase(pointer.SafeDeref(query.Phase)),
		Project:    pointer.SafeDeref(query.Project),
		RemoteAddr: pointer.SafeDeref(query.SourceIp),
		RequestId:  pointer.SafeDeref(query.Uuid),
		StatusCode: code,
		To:         to,
		User:       pointer.SafeDeref(query.User),
	}

	if a.scope != nil {
		filter.Tenant = a.scope.tenantID
	}

	entries, err := a.c.Search(ctx, filter)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("error searching audit backend: %w", err))
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	var result []*auditEntity
	for _, e := range entries {
		switch e.Phase {
		case auditing.EntryPhaseRequest, auditing.EntryPhaseResponse:
		case auditing.EntryPhaseClosed, auditing.EntryPhaseError, auditing.EntryPhaseOpened, auditing.EntryPhaseSingle:
			continue
		default:
			continue
		}

		result = append(result, &auditEntity{Entry: e})
	}

	return result, nil
}

func (a *auditRepository) validateCreate(ctx context.Context, create any) error {
	return nil
}

func (a *auditRepository) validateDelete(ctx context.Context, e *auditEntity) error {
	return nil
}

func (a *auditRepository) validateUpdate(ctx context.Context, msg *auditEntity, _ *auditEntity) error {
	return nil
}

func (a *auditRepository) matchScope(e *auditEntity) bool {
	return true
}

func (a *auditRepository) get(ctx context.Context, id string) (*auditEntity, error) {
	panic("unimplemented")
}

func (a *auditRepository) create(ctx context.Context, rq any) (*auditEntity, error) {
	panic("unimplemented")
}

func (a *auditRepository) find(ctx context.Context, query *apiv2.AuditQuery) (*auditEntity, error) {
	panic("unimplemented")
}

func (a *auditRepository) delete(ctx context.Context, audit *auditEntity) error {
	panic("unimplemented")
}

func (a *auditRepository) update(ctx context.Context, audit *auditEntity, rq *auditEntity) (*auditEntity, error) {
	panic("unimplemented")
}

func (a *auditRepository) convertToInternal(ctx context.Context, audit *apiv2.AuditTrace) (*auditEntity, error) {
	panic("unimplemented")
}

func (a *auditRepository) convertToProto(ctx context.Context, e *auditEntity) (*apiv2.AuditTrace, error) {
	sourceIP := e.ForwardedFor
	if sourceIP == "" {
		sourceIP = e.RemoteAddr
	}

	payload := e.Body
	if payload == nil {
		payload = e.Error
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal audit trace payload: %w", err)
	}
	parsedBody := string(rawPayload)

	var resultCode *int32
	if e.Phase == auditing.EntryPhaseResponse && e.StatusCode != nil {
		resultCode = new(int32(*e.StatusCode))
	}

	return &apiv2.AuditTrace{
		Uuid:       e.RequestId,
		Timestamp:  timestamppb.New(e.Timestamp),
		User:       e.User,
		Tenant:     e.Tenant,
		Project:    pointer.PointerOrNil(e.Project),
		Method:     e.Path,
		Body:       &parsedBody,
		SourceIp:   sourceIP,
		ResultCode: resultCode,
		Phase:      convertToExternalPhase(e.Phase),
	}, nil
}

func (a auditEntity) SetChanged(time time.Time) {}

func (auditEntity) GetUpdateMeta() *apiv2.UpdateMeta {
	return &apiv2.UpdateMeta{}
}

func convertToInternalPhase(phase apiv2.AuditPhase) auditing.EntryPhase {
	switch phase {
	case apiv2.AuditPhase_AUDIT_PHASE_REQUEST:
		return auditing.EntryPhaseRequest
	case apiv2.AuditPhase_AUDIT_PHASE_RESPONSE:
		return auditing.EntryPhaseResponse
	case apiv2.AuditPhase_AUDIT_PHASE_UNSPECIFIED:
		fallthrough
	default:
		return auditing.EntryPhase("")
	}
}

func convertToExternalPhase(phase auditing.EntryPhase) apiv2.AuditPhase {
	switch phase {
	case auditing.EntryPhaseRequest:
		return apiv2.AuditPhase_AUDIT_PHASE_REQUEST
	case auditing.EntryPhaseResponse:
		return apiv2.AuditPhase_AUDIT_PHASE_RESPONSE
	case auditing.EntryPhaseClosed, auditing.EntryPhaseError, auditing.EntryPhaseSingle, auditing.EntryPhaseOpened:
		fallthrough
	default:
		return apiv2.AuditPhase_AUDIT_PHASE_UNSPECIFIED
	}
}
