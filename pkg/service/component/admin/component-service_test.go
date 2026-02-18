package admin

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"
)

func Test_componentServiceServer_List(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testStore, repocloser := test.StartRepositoryWithCleanup(t, log, test.WithContainers(false))
	defer repocloser()

	tests := []struct {
		name    string
		pings   []*adminv2.Component
		req     *adminv2.ComponentServiceListRequest
		want    *adminv2.ComponentServiceListResponse
		wantErr error
	}{
		{
			name: "simple, only one component",
			pings: []*adminv2.Component{
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.5.2"},
				},
			},
			req: &adminv2.ComponentServiceListRequest{},
			want: &adminv2.ComponentServiceListResponse{
				Components: []*adminv2.Component{
					{
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
						Identifier: "management-server-01",
						Version:    &apiv2.Version{Version: "v0.5.2"},
						Interval:   durationpb.New(time.Minute),
					},
				},
			},
		},
		{
			name: "simple, same component two pings within expiration, latest wins",
			pings: []*adminv2.Component{
				{
					Uuid:       "700a2150-b51c-43be-b3e6-d944ed216a2c",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.5.2"},
				},
				{
					Uuid:       "1aaccc66-441f-4914-a088-b61b478200b5",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.5.2"},
				},
			},
			req: &adminv2.ComponentServiceListRequest{},
			want: &adminv2.ComponentServiceListResponse{
				Components: []*adminv2.Component{
					{
						Uuid:       "1aaccc66-441f-4914-a088-b61b478200b5",
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
						Identifier: "management-server-01",
						Version:    &apiv2.Version{Version: "v0.5.2"},
						Interval:   durationpb.New(time.Minute),
					},
				},
			},
		},
		{
			name: "more different components",
			pings: []*adminv2.Component{
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.5.2"},
				},
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CONSOLE,
					Identifier: "control-plane",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.1.2"},
				},
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CORE,
					Identifier: "switch-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.4.0"},
				},
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_IMAGE_CACHE_SYNC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.2"},
				},
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_METRICS_EXPORTER,
					Identifier: "control-plane",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.1"},
				},
			},
			req: &adminv2.ComponentServiceListRequest{},
			want: &adminv2.ComponentServiceListResponse{
				Components: []*adminv2.Component{
					{
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
						Identifier: "management-server-01",
						Version:    &apiv2.Version{Version: "v0.5.2"},
						Interval:   durationpb.New(time.Minute),
					},
					{
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CONSOLE,
						Identifier: "control-plane",
						Interval:   durationpb.New(time.Minute),
						Version:    &apiv2.Version{Version: "v0.1.2"},
					},
					{
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CORE,
						Identifier: "switch-01",
						Interval:   durationpb.New(time.Minute),
						Version:    &apiv2.Version{Version: "v0.4.0"},
					},
					{
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_IMAGE_CACHE_SYNC,
						Identifier: "management-server-01",
						Interval:   durationpb.New(time.Minute),
						Version:    &apiv2.Version{Version: "v0.0.2"},
					},
					{
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_METRICS_EXPORTER,
						Identifier: "control-plane",
						Interval:   durationpb.New(time.Minute),
						Version:    &apiv2.Version{Version: "v0.0.1"},
					},
				},
			},
		},
		{
			name: "query for type",
			pings: []*adminv2.Component{
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.5.2"},
				},
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CONSOLE,
					Identifier: "control-plane",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.1.2"},
				},
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CORE,
					Identifier: "switch-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.4.0"},
				},
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_IMAGE_CACHE_SYNC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.2"},
				},
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_METRICS_EXPORTER,
					Identifier: "control-plane",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.1"},
				},
			},
			req: &adminv2.ComponentServiceListRequest{Query: &adminv2.ComponentQuery{Type: infrav2.ComponentType_COMPONENT_TYPE_METAL_CONSOLE.Enum()}},
			want: &adminv2.ComponentServiceListResponse{
				Components: []*adminv2.Component{
					{
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CONSOLE,
						Identifier: "control-plane",
						Interval:   durationpb.New(time.Minute),
						Version:    &apiv2.Version{Version: "v0.1.2"},
					},
				},
			},
		},
		{
			name: "query for identifier",
			pings: []*adminv2.Component{
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.5.2"},
				},
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CONSOLE,
					Identifier: "control-plane",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.1.2"},
				},
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CORE,
					Identifier: "switch-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.4.0"},
				},
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_IMAGE_CACHE_SYNC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.2"},
				},
				{
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_METRICS_EXPORTER,
					Identifier: "control-plane",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.1"},
				},
			},
			req: &adminv2.ComponentServiceListRequest{Query: &adminv2.ComponentQuery{Identifier: new("control-plane")}},
			want: &adminv2.ComponentServiceListResponse{
				Components: []*adminv2.Component{
					{
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CONSOLE,
						Identifier: "control-plane",
						Interval:   durationpb.New(time.Minute),
						Version:    &apiv2.Version{Version: "v0.1.2"},
					},
					{
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_METRICS_EXPORTER,
						Identifier: "control-plane",
						Interval:   durationpb.New(time.Minute),
						Version:    &apiv2.Version{Version: "v0.0.1"},
					},
				},
			},
		},
		{
			name: "query for uuid",
			pings: []*adminv2.Component{
				{
					Uuid:       "4fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.5.2"},
				},
				{
					Uuid:       "5fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CONSOLE,
					Identifier: "control-plane",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.1.2"},
				},
				{
					Uuid:       "7fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CORE,
					Identifier: "switch-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.4.0"},
				},
				{
					Uuid:       "6fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_IMAGE_CACHE_SYNC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.2"},
				},
				{
					Uuid:       "8fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_METRICS_EXPORTER,
					Identifier: "control-plane",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.1"},
				},
			},
			req: &adminv2.ComponentServiceListRequest{Query: &adminv2.ComponentQuery{Uuid: new("6fda7ab5-7d26-41e5-83b3-51f349ab9877")}},
			want: &adminv2.ComponentServiceListResponse{
				Components: []*adminv2.Component{
					{
						Uuid:       "6fda7ab5-7d26-41e5-83b3-51f349ab9877",
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_IMAGE_CACHE_SYNC,
						Identifier: "management-server-01",
						Interval:   durationpb.New(time.Minute),
						Version:    &apiv2.Version{Version: "v0.0.2"},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &componentServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			for _, ping := range tt.pings {
				_, err := testStore.Store.Component().Create(ctx, ping)
				require.NoError(t, err)
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.req)
			}
			got, err := c.List(ctx, tt.req)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
				return
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				cmpopts.IgnoreUnexported(),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got, tt.want, diff)
			}

		})
	}
}

func Test_componentServiceServer_Get(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testStore, repocloser := test.StartRepositoryWithCleanup(t, log, test.WithContainers(false))
	defer repocloser()

	tests := []struct {
		name    string
		pings   []*adminv2.Component
		req     *adminv2.ComponentServiceGetRequest
		want    *adminv2.ComponentServiceGetResponse
		wantErr error
	}{
		{
			name: "simple, only one component",
			pings: []*adminv2.Component{
				{
					Uuid:       "8fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.5.2"},
				},
			},
			req: &adminv2.ComponentServiceGetRequest{Uuid: "8fda7ab5-7d26-41e5-83b3-51f349ab9877"},
			want: &adminv2.ComponentServiceGetResponse{
				Component: &adminv2.Component{
					Uuid:       "8fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.5.2"},
				},
			},
		},
		{
			name: "not found component",
			pings: []*adminv2.Component{
				{
					Uuid:       "8fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.5.2"},
				},
			},
			req:     &adminv2.ComponentServiceGetRequest{Uuid: "8fda7ab5-7d26-41e5-83b3-51f349ab9876"},
			want:    nil,
			wantErr: errorutil.NotFound("no component with uuid 8fda7ab5-7d26-41e5-83b3-51f349ab9876 found"),
		},
		{
			name: "more different components",
			pings: []*adminv2.Component{
				{
					Uuid:       "8fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_BMC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.5.2"},
				},
				{
					Uuid:       "7fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CONSOLE,
					Identifier: "control-plane",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.1.2"},
				},
				{
					Uuid:       "6fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_CORE,
					Identifier: "switch-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.4.0"},
				},
				{
					Uuid:       "5fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_IMAGE_CACHE_SYNC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.2"},
				},
				{
					Uuid:       "4fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_METRICS_EXPORTER,
					Identifier: "control-plane",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.1"},
				},
			},
			req: &adminv2.ComponentServiceGetRequest{Uuid: "4fda7ab5-7d26-41e5-83b3-51f349ab9877"},
			want: &adminv2.ComponentServiceGetResponse{
				Component: &adminv2.Component{
					Uuid:       "4fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_METRICS_EXPORTER,
					Identifier: "control-plane",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.1"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &componentServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			for _, ping := range tt.pings {
				_, err := testStore.Store.Component().Create(ctx, ping)
				require.NoError(t, err)
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.req)
			}
			got, err := c.Get(ctx, tt.req)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
				return
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				cmpopts.IgnoreUnexported(),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got, tt.want, diff)
			}

		})
	}
}

func Test_componentServiceServer_Delete(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	testStore, repocloser := test.StartRepositoryWithCleanup(t, log, test.WithContainers(false))
	defer repocloser()

	tests := []struct {
		name    string
		pings   []*adminv2.Component
		req     *adminv2.ComponentServiceDeleteRequest
		want    *adminv2.ComponentServiceListResponse
		wantErr error
	}{

		{
			name: "not found",
			pings: []*adminv2.Component{
				{
					Uuid:       "5fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_IMAGE_CACHE_SYNC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.2"},
				},
				{
					Uuid:       "4fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_METRICS_EXPORTER,
					Identifier: "control-plane",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.1"},
				},
			},
			req: &adminv2.ComponentServiceDeleteRequest{Uuid: "2aaccc66-441f-4914-a088-b61b478200b5"},
			want: &adminv2.ComponentServiceListResponse{
				Components: []*adminv2.Component{
					{
						Uuid:       "5fda7ab5-7d26-41e5-83b3-51f349ab9877",
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_IMAGE_CACHE_SYNC,
						Identifier: "management-server-01",
						Interval:   durationpb.New(time.Minute),
						Version:    &apiv2.Version{Version: "v0.0.2"},
					},
					{
						Uuid:       "4fda7ab5-7d26-41e5-83b3-51f349ab9877",
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_METRICS_EXPORTER,
						Identifier: "control-plane",
						Interval:   durationpb.New(time.Minute),
						Version:    &apiv2.Version{Version: "v0.0.1"},
					},
				},
			},
			wantErr: errorutil.NotFound("no component with uuid 2aaccc66-441f-4914-a088-b61b478200b5 found"),
		},
		{
			name: "simple, only one component",
			pings: []*adminv2.Component{
				{
					Uuid:       "5fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_IMAGE_CACHE_SYNC,
					Identifier: "management-server-01",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.2"},
				},
				{
					Uuid:       "4fda7ab5-7d26-41e5-83b3-51f349ab9877",
					Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_METRICS_EXPORTER,
					Identifier: "control-plane",
					Interval:   durationpb.New(time.Minute),
					Version:    &apiv2.Version{Version: "v0.0.1"},
				},
			},
			req: &adminv2.ComponentServiceDeleteRequest{Uuid: "5fda7ab5-7d26-41e5-83b3-51f349ab9877"},
			want: &adminv2.ComponentServiceListResponse{
				Components: []*adminv2.Component{
					{
						Uuid:       "4fda7ab5-7d26-41e5-83b3-51f349ab9877",
						Type:       infrav2.ComponentType_COMPONENT_TYPE_METAL_METRICS_EXPORTER,
						Identifier: "control-plane",
						Interval:   durationpb.New(time.Minute),
						Version:    &apiv2.Version{Version: "v0.0.1"},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &componentServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			for _, ping := range tt.pings {
				_, err := testStore.Store.Component().Create(ctx, ping)
				require.NoError(t, err)
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.req)
			}
			_, err := c.Delete(ctx, tt.req)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
				return
			}
			got, err := c.List(ctx, &adminv2.ComponentServiceListRequest{})
			require.NoError(t, err)
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				cmpopts.IgnoreUnexported(),
			); diff != "" {
				t.Errorf("%v, want %v diff: %s", got, tt.want, diff)
			}

		})
	}
}
