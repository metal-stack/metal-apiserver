package adminaudit

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/auditing"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Test_auditServiceServer_Get(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	now := time.Now()

	tests := []struct {
		name    string
		rq      *adminv2.AuditServiceGetRequest
		entries []auditing.Entry
		want    *adminv2.AuditServiceGetResponse
		wantErr error
	}{
		{
			name:    "get non existing",
			rq:      &adminv2.AuditServiceGetRequest{Uuid: "99d84f08-85f3-4d4e-881c-29c2c9e1ba58"},
			want:    nil,
			wantErr: errorutil.NotFound(`no audit trace found for given request id "99d84f08-85f3-4d4e-881c-29c2c9e1ba58"`),
		},
		{
			name: "get existing defaults to request phase",
			entries: []auditing.Entry{
				{
					Component:    api.AuditingComponent,
					RequestId:    "99d84f08-85f3-4d4e-881c-29c2c9e1ba58",
					Type:         auditing.EntryTypeGRPC,
					Timestamp:    now,
					User:         "foo",
					Tenant:       "a-tenant",
					Project:      "b",
					Detail:       auditing.EntryDetailGRPCUnary,
					Phase:        auditing.EntryPhaseRequest,
					Path:         "/a/path/",
					ForwardedFor: "1.2.3.4",
					RemoteAddr:   "2.3.4.5",
					Body:         "a body",
					Error:        nil,
				},
			},
			rq: &adminv2.AuditServiceGetRequest{Uuid: "99d84f08-85f3-4d4e-881c-29c2c9e1ba58"},
			want: &adminv2.AuditServiceGetResponse{
				Trace: &apiv2.AuditTrace{
					Uuid:      "99d84f08-85f3-4d4e-881c-29c2c9e1ba58",
					Timestamp: timestamppb.New(now),
					User:      "foo",
					Tenant:    "a-tenant",
					Project:   new("b"),
					Method:    "/a/path/",
					Body:      new(`"a body"`),
					SourceIp:  "1.2.3.4",
					Phase:     apiv2.AuditPhase_AUDIT_PHASE_REQUEST,
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := auditing.NewMemory(auditing.Config{
				Component: api.AuditingComponent,
				Log:       log,
			}, auditing.MemoryConfig{})
			require.NoError(t, err)

			for _, e := range tt.entries {
				require.NoError(t, c.Index(e))
			}

			s := &auditServiceServer{
				log:  log,
				repo: repository.New(repository.Config{Log: log, Auditing: c}).UnscopedAudit(),
			}

			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}

			got, err := s.Get(t.Context(), tt.rq)

			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("Get() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_auditServiceServer_List(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	now := time.Now()

	tests := []struct {
		name    string
		rq      *adminv2.AuditServiceListRequest
		entries []auditing.Entry
		want    *adminv2.AuditServiceListResponse
		wantErr error
	}{
		{
			name: "list",
			entries: []auditing.Entry{
				{
					Component:    api.AuditingComponent,
					RequestId:    "99d84f08-85f3-4d4e-881c-29c2c9e1ba58",
					Type:         auditing.EntryTypeGRPC,
					Timestamp:    now,
					User:         "foo",
					Tenant:       "a-tenant",
					Project:      "b",
					Detail:       auditing.EntryDetailGRPCUnary,
					Phase:        auditing.EntryPhaseRequest,
					Path:         "/a/path/",
					ForwardedFor: "1.2.3.4",
					RemoteAddr:   "2.3.4.5",
					Body:         "a request body",
					Error:        nil,
				},
				{
					Component:    api.AuditingComponent,
					RequestId:    "99d84f08-85f3-4d4e-881c-29c2c9e1ba58",
					Type:         auditing.EntryTypeGRPC,
					Timestamp:    now,
					User:         "foo",
					Tenant:       "a-tenant",
					Project:      "b",
					Detail:       auditing.EntryDetailGRPCUnary,
					Phase:        auditing.EntryPhaseResponse,
					Path:         "/a/path/",
					ForwardedFor: "1.2.3.4",
					RemoteAddr:   "2.3.4.5",
					Body:         "a response body",
					Error:        nil,
				},
				{
					Component:    api.AuditingComponent,
					RequestId:    "c7c60cc9-e47d-4c7a-bd2d-b65dd4f0a59c",
					Type:         auditing.EntryTypeGRPC,
					Timestamp:    now,
					User:         "foo",
					Tenant:       "another-tenant",
					Project:      "b",
					Detail:       auditing.EntryDetailGRPCUnary,
					Phase:        auditing.EntryPhaseResponse,
					Path:         "/a/path/",
					ForwardedFor: "1.2.3.4",
					RemoteAddr:   "2.3.4.5",
					Body:         "a response body",
					Error:        nil,
				},
			},
			rq: &adminv2.AuditServiceListRequest{},
			want: &adminv2.AuditServiceListResponse{
				Traces: []*apiv2.AuditTrace{
					{
						Uuid:      "99d84f08-85f3-4d4e-881c-29c2c9e1ba58",
						Timestamp: timestamppb.New(now),
						User:      "foo",
						Tenant:    "a-tenant",
						Project:   new("b"),
						Method:    "/a/path/",
						Body:      new(`"a request body"`),
						SourceIp:  "1.2.3.4",
						Phase:     apiv2.AuditPhase_AUDIT_PHASE_REQUEST,
					},
					{
						Uuid:      "99d84f08-85f3-4d4e-881c-29c2c9e1ba58",
						Timestamp: timestamppb.New(now),
						User:      "foo",
						Tenant:    "a-tenant",
						Project:   new("b"),
						Method:    "/a/path/",
						Body:      new(`"a response body"`),
						SourceIp:  "1.2.3.4",
						Phase:     apiv2.AuditPhase_AUDIT_PHASE_RESPONSE,
					},
					{
						Uuid:      "c7c60cc9-e47d-4c7a-bd2d-b65dd4f0a59c",
						Timestamp: timestamppb.New(now),
						User:      "foo",
						Tenant:    "another-tenant",
						Project:   new("b"),
						Method:    "/a/path/",
						Body:      new(`"a response body"`),
						SourceIp:  "1.2.3.4",
						Phase:     apiv2.AuditPhase_AUDIT_PHASE_RESPONSE,
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := auditing.NewMemory(auditing.Config{
				Component: api.AuditingComponent,
				Log:       log,
			}, auditing.MemoryConfig{})
			require.NoError(t, err)

			for _, e := range tt.entries {
				require.NoError(t, c.Index(e))
			}

			s := &auditServiceServer{
				log:  log,
				repo: repository.New(repository.Config{Log: log, Auditing: c}).UnscopedAudit(),
			}

			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}

			got, err := s.List(t.Context(), tt.rq)

			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
			); diff != "" {
				t.Errorf("List() = %v, want %v diff: %s", got, tt.want, diff)
			}
		})
	}
}
