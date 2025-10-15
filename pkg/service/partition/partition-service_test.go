package partition

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_partitionServiceServer_Get(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})

	tests := []struct {
		name    string
		rq      *apiv2.PartitionServiceGetRequest
		want    *apiv2.PartitionServiceGetResponse
		wantErr error
	}{
		{
			name:    "get existing",
			rq:      &apiv2.PartitionServiceGetRequest{Id: "partition-1"},
			want:    &apiv2.PartitionServiceGetResponse{Partition: &apiv2.Partition{Id: "partition-1", Meta: &apiv2.Meta{}, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			wantErr: nil,
		},
		{
			name:    "get non existing",
			rq:      &apiv2.PartitionServiceGetRequest{Id: "nonexisting"},
			want:    nil,
			wantErr: errorutil.NotFound(`no partition with id "nonexisting" found`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &partitionServiceServer{
				log:  log,
				repo: repo,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := p.Get(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.IP{}, "uuid",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("partitionServiceServer.Get() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_partitionServiceServer_List(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
		{
			Partition: &apiv2.Partition{Id: "partition-2", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})

	tests := []struct {
		name    string
		rq      *apiv2.PartitionServiceListRequest
		want    *apiv2.PartitionServiceListResponse
		wantErr error
	}{
		{
			name: "list one existing",
			rq:   &apiv2.PartitionServiceListRequest{Query: &apiv2.PartitionQuery{Id: pointer.Pointer("partition-1")}},
			want: &apiv2.PartitionServiceListResponse{Partitions: []*apiv2.Partition{
				{Id: "partition-1", Meta: &apiv2.Meta{}, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			},
			},
			wantErr: nil,
		},
		{
			name: "list all",
			rq:   &apiv2.PartitionServiceListRequest{},
			want: &apiv2.PartitionServiceListResponse{Partitions: []*apiv2.Partition{
				{Id: "partition-1", Meta: &apiv2.Meta{}, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
				{Id: "partition-2", Meta: &apiv2.Meta{}, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			},
			},
			wantErr: nil,
		},
		{
			name:    "list non existing",
			rq:      &apiv2.PartitionServiceListRequest{Query: &apiv2.PartitionQuery{Id: pointer.Pointer("partition-3")}},
			want:    &apiv2.PartitionServiceListResponse{},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &partitionServiceServer{
				log:  log,
				repo: repo,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.rq)
			}
			got, err := p.List(ctx, connect.NewRequest(tt.rq))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}

			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.IP{}, "uuid",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("partitionServiceServer.List() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}
