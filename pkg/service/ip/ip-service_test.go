package ip

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"slices"
	"testing"

	"connectrpc.com/connect"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/metal"
	"github.com/metal-stack/api-server/pkg/db/repository"
	"github.com/metal-stack/api-server/pkg/test"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	mdmv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdmock "github.com/metal-stack/masterdata-api/api/v1/mocks"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/redis/go-redis/v9"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

var prefixMap = map[string][]string{
	"1.2.3.0/24":    {"1.2.3.4", "1.2.3.5", "1.2.3.6"},
	"2.3.4.0/24":    {"2.3.4.5"},
	"2001:db8::/96": {"2001:db8::1"},
}

func Test_ipServiceServer_Get(t *testing.T) {
	container, c, err := test.StartRethink(t)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(context.Background())
	}()
	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})

	ipam := test.StartIpam(t)

	ctx := context.Background()
	log := slog.Default()

	ds, err := generic.New(log, "metal", c)
	require.NoError(t, err)

	repo, err := repository.New(log, nil, ds, ipam, rc)
	require.NoError(t, err)

	createIPs(t, ctx, ds, ipam, prefixMap, []*metal.IP{{IPAddress: "1.2.3.4", ProjectID: "p1"}})

	require.NoError(t, err)

	tests := []struct {
		name           string
		log            *slog.Logger
		ctx            context.Context
		rq             *apiv2.IPServiceGetRequest
		ds             *generic.Datastore
		want           *apiv2.IPServiceGetResponse
		wantReturnCode connect.Code
		wantErr        bool
	}{
		{
			name:    "get existing",
			log:     log,
			ctx:     ctx,
			rq:      &apiv2.IPServiceGetRequest{Ip: "1.2.3.4", Project: "p1"},
			ds:      ds,
			want:    &apiv2.IPServiceGetResponse{Ip: &apiv2.IP{Ip: "1.2.3.4", Project: "p1"}},
			wantErr: false,
		},
		{
			name:    "get non existing",
			log:     log,
			ctx:     ctx,
			rq:      &apiv2.IPServiceGetRequest{Ip: "1.2.3.5"},
			ds:      ds,
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  tt.log,
				repo: repo,
			}
			got, err := i.Get(tt.ctx, connect.NewRequest(tt.rq))
			if (err != nil) != tt.wantErr {
				t.Errorf("ipServiceServer.Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want == nil && got == nil {
				return
			}
			if tt.want == nil && got != nil {
				t.Error("tt.want is nil but got is not")
				return
			}
			if diff := cmp.Diff(
				tt.want, got.Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.IP{}, "created_at", "updated_at", "uuid",
					),
				},
			); diff != "" {
				t.Errorf("ipServiceServer.Get() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_ipServiceServer_List(t *testing.T) {
	container, c, err := test.StartRethink(t)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(context.Background())
	}()
	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})

	ipam := test.StartIpam(t)

	ctx := context.Background()
	log := slog.Default()

	ds, err := generic.New(log, "metal", c)
	require.NoError(t, err)

	repo, err := repository.New(log, nil, ds, ipam, rc)
	require.NoError(t, err)

	ips := []*metal.IP{
		{Name: "ip1", IPAddress: "1.2.3.4", ProjectID: "p1"},
		{Name: "ip2", IPAddress: "1.2.3.5", ProjectID: "p1"},
		{Name: "ip3", IPAddress: "1.2.3.6", ProjectID: "p1", NetworkID: "n1"},
		{Name: "ip4", IPAddress: "2001:db8::1", ProjectID: "p2", NetworkID: "n2"},
		{Name: "ip5", IPAddress: "2.3.4.5", ProjectID: "p2", NetworkID: "n3", ParentPrefixCidr: "2.3.4.0/24"},
	}
	createIPs(t, ctx, ds, ipam, prefixMap, ips)

	tests := []struct {
		name           string
		log            *slog.Logger
		ctx            context.Context
		rq             *apiv2.IPServiceListRequest
		ds             *generic.Datastore
		want           *apiv2.IPServiceListResponse
		wantReturnCode connect.Code
		wantErr        bool
	}{
		{
			name:    "get by ip",
			log:     log,
			ctx:     ctx,
			rq:      &apiv2.IPServiceListRequest{Ip: pointer.Pointer("1.2.3.4"), Project: "p1"},
			ds:      ds,
			want:    &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{{Name: "ip1", Ip: "1.2.3.4", Project: "p1"}}},
			wantErr: false,
		},
		{
			name:    "get by project",
			log:     log,
			ctx:     ctx,
			rq:      &apiv2.IPServiceListRequest{Project: "p1"},
			ds:      ds,
			want:    &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{{Name: "ip1", Ip: "1.2.3.4", Project: "p1"}, {Name: "ip2", Ip: "1.2.3.5", Project: "p1"}, {Name: "ip3", Ip: "1.2.3.6", Project: "p1", Network: "n1"}}},
			wantErr: false,
		},
		{
			name:    "get by addressfamily",
			log:     log,
			ctx:     ctx,
			rq:      &apiv2.IPServiceListRequest{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum(), Project: "p2"},
			ds:      ds,
			want:    &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{{Name: "ip4", Ip: "2001:db8::1", Project: "p2", Network: "n2"}}},
			wantErr: false,
		},
		{
			name:    "get by parent prefix cidr",
			log:     log,
			ctx:     ctx,
			rq:      &apiv2.IPServiceListRequest{ParentPrefixCidr: pointer.Pointer("2.3.4.0/24"), Project: "p2"},
			ds:      ds,
			want:    &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{{Name: "ip5", Ip: "2.3.4.5", Project: "p2", Network: "n3"}}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  tt.log,
				repo: repo,
			}
			got, err := i.List(tt.ctx, connect.NewRequest(tt.rq))
			if (err != nil) != tt.wantErr {
				t.Errorf("ipServiceServer.List() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want == nil && got == nil {
				return
			}
			if tt.want == nil && got != nil {
				t.Error("tt.want is nil but got is not")
				return
			}
			if diff := cmp.Diff(
				tt.want, got.Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.IP{}, "created_at", "updated_at", "uuid",
					),
				},
			); diff != "" {
				t.Errorf("ipServiceServer.List() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_ipServiceServer_Update(t *testing.T) {
	container, c, err := test.StartRethink(t)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(context.Background())
	}()
	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})

	ipam := test.StartIpam(t)

	ctx := context.Background()
	log := slog.Default()

	ds, err := generic.New(log, "metal", c)
	require.NoError(t, err)

	repo, err := repository.New(log, nil, ds, ipam, rc)
	require.NoError(t, err)

	ips := []*metal.IP{
		{Name: "ip1", IPAddress: "1.2.3.4", ProjectID: "p1"},
		{Name: "ip2", IPAddress: "1.2.3.5", ProjectID: "p1"},
		{Name: "ip3", IPAddress: "1.2.3.6", ProjectID: "p1", NetworkID: "n1"},
		{Name: "ip4", IPAddress: "2001:db8::1", ProjectID: "p2", NetworkID: "n2", Tags: []string{"color=red"}},
		{Name: "ip5", IPAddress: "2.3.4.5", ProjectID: "p2", NetworkID: "n3", ParentPrefixCidr: "2.3.4.0/24"},
	}
	createIPs(t, ctx, ds, ipam, prefixMap, ips)

	tests := []struct {
		name           string
		log            *slog.Logger
		ctx            context.Context
		rq             *apiv2.IPServiceUpdateRequest
		ds             *generic.Datastore
		want           *apiv2.IPServiceUpdateResponse
		wantReturnCode connect.Code
		wantErr        bool
	}{
		{
			name:    "update name",
			log:     log,
			ctx:     ctx,
			rq:      &apiv2.IPServiceUpdateRequest{Ip: "1.2.3.4", Project: "p1", Name: pointer.Pointer("ip1-changed")},
			ds:      ds,
			want:    &apiv2.IPServiceUpdateResponse{Ip: &apiv2.IP{Name: "ip1-changed", Ip: "1.2.3.4", Project: "p1"}},
			wantErr: false,
		},
		{
			name:    "update description",
			log:     log,
			ctx:     ctx,
			rq:      &apiv2.IPServiceUpdateRequest{Ip: "1.2.3.5", Project: "p1", Description: pointer.Pointer("test was here")},
			ds:      ds,
			want:    &apiv2.IPServiceUpdateResponse{Ip: &apiv2.IP{Name: "ip2", Ip: "1.2.3.5", Project: "p1", Description: "test was here"}},
			wantErr: false,
		},
		{
			name:    "update type",
			log:     log,
			ctx:     ctx,
			rq:      &apiv2.IPServiceUpdateRequest{Ip: "1.2.3.6", Project: "p1", Type: apiv2.IPType_IP_TYPE_STATIC.Enum()},
			ds:      ds,
			want:    &apiv2.IPServiceUpdateResponse{Ip: &apiv2.IP{Name: "ip3", Ip: "1.2.3.6", Project: "p1", Network: "n1", Type: apiv2.IPType_IP_TYPE_STATIC}},
			wantErr: false,
		},
		{
			name:    "update tags",
			log:     log,
			ctx:     ctx,
			rq:      &apiv2.IPServiceUpdateRequest{Ip: "2001:db8::1", Project: "p2", Tags: []string{"color=red", "purpose=lb"}},
			ds:      ds,
			want:    &apiv2.IPServiceUpdateResponse{Ip: &apiv2.IP{Name: "ip4", Ip: "2001:db8::1", Project: "p2", Network: "n2", Tags: []string{"color=red", "purpose=lb"}}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  tt.log,
				repo: repo,
			}
			got, err := i.Update(tt.ctx, connect.NewRequest(tt.rq))
			if (err != nil) != tt.wantErr {
				t.Errorf("ipServiceServer.Update() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want == nil && got == nil {
				return
			}
			if tt.want == nil && got != nil {
				t.Error("tt.want is nil but got is not")
				return
			}
			if diff := cmp.Diff(
				tt.want, got.Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.IP{}, "created_at", "updated_at", "uuid",
					),
				},
			); diff != "" {
				t.Errorf("ipServiceServer.Update() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_ipServiceServer_Delete(t *testing.T) {
	ctx := context.Background()
	container, c, err := test.StartRethink(t)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(context.Background())
	}()
	container, rc, err := test.StartValkey(t, ctx)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(ctx)
	}()

	ipam := test.StartIpam(t)

	log := slog.Default()

	ds, err := generic.New(log, "metal", c)
	require.NoError(t, err)

	repo, err := repository.New(log, nil, ds, ipam, rc)
	require.NoError(t, err)

	ips := []*metal.IP{
		{Name: "ip1", IPAddress: "1.2.3.4", ProjectID: "p1", ParentPrefixCidr: "1.2.3.0/24", AllocationUUID: uuid.NewString()},
		{Name: "ip2", IPAddress: "1.2.3.5", ProjectID: "p1", ParentPrefixCidr: "1.2.3.0/24", AllocationUUID: uuid.NewString()},
		{Name: "ip3", IPAddress: "1.2.3.6", ProjectID: "p1", NetworkID: "n1", ParentPrefixCidr: "1.2.3.0/24", AllocationUUID: uuid.NewString()},
		{Name: "ip4", IPAddress: "2001:db8::1", ProjectID: "p2", NetworkID: "n2", ParentPrefixCidr: "2001:db8::/64", AllocationUUID: uuid.NewString()},
		{Name: "ip5", IPAddress: "2.3.4.5", ProjectID: "p2", NetworkID: "n3", ParentPrefixCidr: "2.3.4.0/24", AllocationUUID: uuid.NewString()},
	}
	createIPs(t, ctx, ds, ipam, prefixMap, ips)

	tests := []struct {
		name           string
		log            *slog.Logger
		ctx            context.Context
		rq             *apiv2.IPServiceDeleteRequest
		ds             *generic.Datastore
		want           *apiv2.IPServiceDeleteResponse
		wantReturnCode connect.Code
		wantErr        bool
	}{
		{
			name:    "delete known ip",
			log:     log,
			ctx:     ctx,
			rq:      &apiv2.IPServiceDeleteRequest{Ip: "1.2.3.4", Project: "p1"},
			ds:      ds,
			want:    &apiv2.IPServiceDeleteResponse{Ip: &apiv2.IP{Name: "ip1", Ip: "1.2.3.4", Project: "p1"}},
			wantErr: false,
		},
		{
			name:           "delete unknown ip",
			log:            log,
			ctx:            ctx,
			rq:             &apiv2.IPServiceDeleteRequest{Ip: "1.2.3.7", Project: "p1"},
			ds:             ds,
			want:           nil,
			wantErr:        true,
			wantReturnCode: connect.CodeNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  tt.log,
				repo: repo,
			}
			got, err := i.Delete(tt.ctx, connect.NewRequest(tt.rq))
			if (err != nil) != tt.wantErr {
				t.Errorf("ipServiceServer.Delete() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (err != nil) && tt.wantErr {
				var connectErr *connect.Error
				if errors.As(err, &connectErr) && tt.wantReturnCode != connectErr.Code() {
					t.Errorf("ipServiceServer.Delete() errcode = %v, wantReturnCode %v", connectErr.Code(), tt.wantReturnCode)
				}
			}
			if tt.want == nil && got == nil {
				return
			}
			if tt.want == nil && got != nil {
				t.Error("tt.want is nil but got is not")
				return
			}
			if diff := cmp.Diff(
				tt.want, got.Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.IP{}, "created_at", "updated_at", "uuid",
					),
				},
			); diff != "" {
				t.Errorf("ipServiceServer.Delete() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_ipServiceServer_Create(t *testing.T) {
	container, c, err := test.StartRethink(t)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(context.Background())
	}()
	r := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: r.Addr()})

	ipam := test.StartIpam(t)

	ctx := context.Background()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ds, err := generic.New(log, "metal", c)
	require.NoError(t, err)

	psc := mdmock.ProjectServiceClient{}
	psc.On("Get", testifymock.Anything, &mdmv1.ProjectGetRequest{Id: "p2"}).Return(&mdmv1.ProjectResponse{
		Project: &mdmv1.Project{
			Meta: &mdmv1.Meta{Id: "p2"},
		}}, nil)
	psc.On("Get", testifymock.Anything, &mdmv1.ProjectGetRequest{Id: "p1"}).Return(&mdmv1.ProjectResponse{
		Project: &mdmv1.Project{
			Meta: &mdmv1.Meta{Id: "p1"},
		}}, nil)
	tsc := mdmock.TenantServiceClient{}

	mdc := mdm.NewMock(&psc, &tsc, nil, nil)

	repo, err := repository.New(log, mdc, ds, ipam, rc)
	require.NoError(t, err)

	ips := []*metal.IP{
		{Name: "ip1", IPAddress: "1.2.3.4", ProjectID: "p1"},
		{Name: "ip2", IPAddress: "1.2.3.5", ProjectID: "p1"},
		{Name: "ip3", IPAddress: "1.2.3.6", ProjectID: "p1", NetworkID: "n1"},
		{Name: "ip4", IPAddress: "2001:db8::1", ProjectID: "p2", NetworkID: "n2"},
		{Name: "ip5", IPAddress: "2.3.4.5", ProjectID: "p2", NetworkID: "n3", ParentPrefixCidr: "2.3.4.0/24"},
	}
	nws := []*apiv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.0.0/24"}},
		{Id: pointer.Pointer("tenant-network"), Prefixes: []string{"10.2.0.0/24"}, Options: &apiv2.NetworkOptions{PrivateSuper: true}},
		{Id: pointer.Pointer("tenant-network-v6"), Prefixes: []string{"2001:db8:1::/64"}, Options: &apiv2.NetworkOptions{PrivateSuper: true}},
		{Id: pointer.Pointer("tenant-network-dualstack"), Prefixes: []string{"10.3.0.0/24", "2001:db8:2::/64"}, Options: &apiv2.NetworkOptions{PrivateSuper: true}},
	}
	createNetworks(t, ctx, repo, nws)
	createIPs(t, ctx, ds, ipam, prefixMap, ips)

	tests := []struct {
		name           string
		ctx            context.Context
		rq             *apiv2.IPServiceCreateRequest
		want           *apiv2.IPServiceCreateResponse
		wantErr        bool
		wantReturnCode connect.Code
		wantErrMessage string
	}{
		{
			name: "create random ephemeral ipv4",
			ctx:  ctx,
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "1.2.0.1", Network: "internet", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL},
			},
		},
		{
			name: "create random ephemeral ipv6",
			ctx:  ctx,
			rq: &apiv2.IPServiceCreateRequest{
				Network: "tenant-network-v6",
				Project: "p1",
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "2001:db8:1::1", Network: "tenant-network-v6", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL},
			},
		},
		{
			name: "create specific ephemeral ipv6",
			ctx:  ctx,
			rq: &apiv2.IPServiceCreateRequest{
				Network: "tenant-network-v6",
				Project: "p1",
				Ip:      pointer.Pointer("2001:db8:1::99"),
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "2001:db8:1::99", Network: "tenant-network-v6", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL},
			},
		},
		{
			name: "create random ephemeral ipv4 from a dualstack network",
			ctx:  ctx,
			rq: &apiv2.IPServiceCreateRequest{
				Network: "tenant-network-dualstack",
				Project: "p1",
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "10.3.0.1", Network: "tenant-network-dualstack", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL},
			},
		},
		{
			name: "create random ephemeral ipv6 from a dualstack network",
			ctx:  ctx,
			rq: &apiv2.IPServiceCreateRequest{
				Network:       "tenant-network-dualstack",
				Project:       "p1",
				AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum(),
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "2001:db8:2::1", Network: "tenant-network-dualstack", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL},
			},
		},
		{
			name: "create specific ephemeral ipv4",
			ctx:  ctx,
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
				Ip:      pointer.Pointer("1.2.0.99"),
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "1.2.0.99", Network: "internet", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL},
			},
		},
		{
			name: "create specific static ipv4",
			ctx:  ctx,
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
				Ip:      pointer.Pointer("1.2.0.100"),
				Type:    apiv2.IPType_IP_TYPE_STATIC.Enum(),
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "1.2.0.100", Network: "internet", Project: "p1", Type: apiv2.IPType_IP_TYPE_STATIC},
			},
		},
		{
			name: "create specific ipv4 which is already allocated",
			ctx:  ctx,
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
				Ip:      pointer.Pointer("1.2.0.1"),
			},
			want:           nil,
			wantErr:        true,
			wantReturnCode: connect.CodeInternal, // FIXME should be InvalidArgument
			wantErrMessage: "internal: internal: Conflict ip already allocated",
		},
		{
			name: "allocate a static specific ip outside prefix",
			ctx:  ctx,
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
				Ip:      pointer.Pointer("1.3.0.1"),
			},
			want:           nil,
			wantErr:        true,
			wantReturnCode: connect.CodeInternal, // FIXME should be InvalidArgument
			wantErrMessage: "internal: internal: specific ip not contained in any of the defined prefixes",
		},
		{
			name: "allocate a random ip with unavailable addressfamily",
			ctx:  ctx,
			rq: &apiv2.IPServiceCreateRequest{
				Network:       "tenant-network-v6",
				Project:       "p1",
				AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.Enum(),
			},
			want:           nil,
			wantErr:        true,
			wantReturnCode: connect.CodeInternal,
			wantErrMessage: "internal: invalid_argument: there is no prefix for the given addressfamily:IPv4 present in network:tenant-network-v6 [IPv6]",
		},
		{
			name: "allocate a random ip with unavailable addressfamily",
			ctx:  ctx,
			rq: &apiv2.IPServiceCreateRequest{
				Network:       "tenant-network",
				Project:       "p1",
				AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum(),
			},
			want:           nil,
			wantErr:        true,
			wantReturnCode: connect.CodeInternal,
			wantErrMessage: "internal: invalid_argument: there is no prefix for the given addressfamily:IPv6 present in network:tenant-network [IPv4]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.Create(tt.ctx, connect.NewRequest(tt.rq))
			if (err != nil) != tt.wantErr {
				t.Errorf("ipServiceServer.Create() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (err != nil) && tt.wantErr {
				require.Equal(t, tt.wantErrMessage, err.Error())
				var connectErr *connect.Error
				if errors.As(err, &connectErr) && tt.wantReturnCode != connectErr.Code() {
					t.Errorf("ipServiceServer.Create() errcode = %v, wantReturnCode %v", connectErr.Code(), tt.wantReturnCode)
				}
				return
			}

			if diff := cmp.Diff(
				tt.want, got.Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.IP{}, "created_at", "updated_at", "uuid",
					),
				},
			); diff != "" {
				t.Errorf("ipServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

// FIXME use repository
func createIPs(t *testing.T, ctx context.Context, ds *generic.Datastore, ipam ipamv1connect.IpamServiceClient, prefixesMap map[string][]string, ips []*metal.IP) {
	for prefix := range prefixesMap {
		_, err := ipam.CreatePrefix(ctx, connect.NewRequest(&ipamv1.CreatePrefixRequest{Cidr: prefix}))
		require.NoError(t, err)
	}
	for _, ip := range ips {
		created, err := ds.IP().Create(ctx, &metal.IP{
			Name: ip.Name, IPAddress: ip.IPAddress,
			ProjectID: ip.ProjectID, AllocationUUID: ip.AllocationUUID,
			ParentPrefixCidr: ip.ParentPrefixCidr, Description: ip.Description,
			NetworkID: ip.NetworkID, Type: ip.Type, Tags: ip.Tags,
		})
		require.NoError(t, err)

		var prefix string
		for pfx, newIPs := range prefixesMap {
			if slices.Contains(newIPs, ip.IPAddress) {
				prefix = pfx
			}
		}

		_, err = ipam.AcquireIP(ctx, connect.NewRequest(&ipamv1.AcquireIPRequest{Ip: &created.IPAddress, PrefixCidr: prefix}))
		require.NoError(t, err)
	}
}

func createNetworks(t *testing.T, ctx context.Context, repo *repository.Repostore, nws []*apiv2.NetworkServiceCreateRequest) {
	for _, nw := range nws {
		// TODO do not care about project here
		_, err := repo.Network(nil).Create(ctx, nw)
		require.NoError(t, err)
	}
}
