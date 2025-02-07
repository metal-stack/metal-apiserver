package ip

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/metal"
	"github.com/metal-stack/api-server/pkg/repository"
	"github.com/metal-stack/api-server/pkg/test"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	ipamv1 "github.com/metal-stack/go-ipam/api/v1"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	mdmv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdmock "github.com/metal-stack/masterdata-api/api/v1/mocks"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/metal-lib/pkg/pointer"
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

	ipam := test.StartIpam(t)

	ctx := context.Background()
	log := slog.Default()

	ds, err := generic.New(log, "metal", c)
	require.NoError(t, err)

	repo := repository.New(log, nil, ds, ipam)

	createIPs(t, ctx, ds, ipam, prefixMap, []*metal.IP{{IPAddress: "1.2.3.4"}})

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
			rq:      &apiv2.IPServiceGetRequest{Ip: "1.2.3.4"},
			ds:      ds,
			want:    &apiv2.IPServiceGetResponse{Ip: &apiv2.IP{Ip: "1.2.3.4"}},
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
						&apiv2.IP{}, "created_at", "updated_at",
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

	ipam := test.StartIpam(t)

	ctx := context.Background()
	log := slog.Default()

	ds, err := generic.New(log, "metal", c)
	require.NoError(t, err)

	repo := repository.New(log, nil, ds, ipam)

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
						&apiv2.IP{}, "created_at", "updated_at",
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

	ipam := test.StartIpam(t)

	ctx := context.Background()
	log := slog.Default()

	ds, err := generic.New(log, "metal", c)
	require.NoError(t, err)

	repo := repository.New(log, nil, ds, ipam)

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
						&apiv2.IP{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("ipServiceServer.Update() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_ipServiceServer_Delete(t *testing.T) {
	container, c, err := test.StartRethink(t)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(context.Background())
	}()

	ipam := test.StartIpam(t)

	ctx := context.Background()
	log := slog.Default()

	ds, err := generic.New(log, "metal", c)
	require.NoError(t, err)

	repo := repository.New(log, nil, ds, ipam)

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
						&apiv2.IP{}, "created_at", "updated_at",
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

	ipam := test.StartIpam(t)

	ctx := context.Background()
	log := slog.Default()

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

	repo := repository.New(log, mdc, ds, ipam)

	ips := []*metal.IP{
		{Name: "ip1", IPAddress: "1.2.3.4", ProjectID: "p1"},
		{Name: "ip2", IPAddress: "1.2.3.5", ProjectID: "p1"},
		{Name: "ip3", IPAddress: "1.2.3.6", ProjectID: "p1", NetworkID: "n1"},
		{Name: "ip4", IPAddress: "2001:db8::1", ProjectID: "p2", NetworkID: "n2"},
		{Name: "ip5", IPAddress: "2.3.4.5", ProjectID: "p2", NetworkID: "n3", ParentPrefixCidr: "2.3.4.0/24"},
	}
	nws := []*metal.Network{
		{Base: metal.Base{ID: "internet"}, Prefixes: metal.Prefixes{metal.Prefix{IP: "1.2.0.0", Length: "24"}}},
		{Base: metal.Base{ID: "tenant-network"}, PrivateSuper: true, Prefixes: metal.Prefixes{metal.Prefix{IP: "10.2.0.0", Length: "24"}}},
	}
	createNetworks(t, ctx, ds, ipam, nws)
	createIPs(t, ctx, ds, ipam, prefixMap, ips)

	tests := []struct {
		name    string
		ctx     context.Context
		rq      *apiv2.IPServiceCreateRequest
		log     *slog.Logger
		repo    *repository.Repository
		want    *apiv2.IPServiceCreateResponse
		wantErr bool
	}{
		{
			name: "create random ipv4",
			ctx:  ctx,
			log:  log,
			repo: repo,
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "1.2.0.1", Network: "internet", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  tt.log,
				repo: tt.repo,
			}
			got, err := i.Create(tt.ctx, connect.NewRequest(tt.rq))
			if (err != nil) != tt.wantErr {
				t.Errorf("ipServiceServer.Create() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if diff := cmp.Diff(
				tt.want, got.Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.IP{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("ipServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

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

func createNetworks(t *testing.T, ctx context.Context, ds *generic.Datastore, ipam ipamv1connect.IpamServiceClient, nws []*metal.Network) {
	for _, nw := range nws {
		fmt.Printf("create network %s", nw.ID)
		_, err := ds.Network().Create(ctx, nw)
		require.NoError(t, err)

		for _, prefix := range nw.Prefixes {
			_, err = ipam.CreatePrefix(ctx, connect.NewRequest(&ipamv1.CreatePrefixRequest{Cidr: prefix.String()}))
			require.NoError(t, err)
		}
	}
}
