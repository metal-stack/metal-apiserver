package ip

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	mdmv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdmock "github.com/metal-stack/masterdata-api/api/v1/mocks"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

func Test_ipServiceServer_Get(t *testing.T) {
	log := slog.Default()

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

	repo, container := test.StartRepository(t, log, mdc)
	defer func() {
		_ = container.Terminate(context.Background())
	}()
	ctx := context.Background()

	createNetworks(t, ctx, repo, []*apiv2.NetworkServiceCreateRequest{{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.3.0/24"}}})
	createIPs(t, ctx, repo, []*apiv2.IPServiceCreateRequest{{Ip: pointer.Pointer("1.2.3.4"), Project: "p1", Network: "internet"}})

	tests := []struct {
		name           string
		rq             *apiv2.IPServiceGetRequest
		want           *apiv2.IPServiceGetResponse
		wantReturnCode connect.Code
		wantErr        bool
	}{
		{
			name:    "get existing",
			rq:      &apiv2.IPServiceGetRequest{Ip: "1.2.3.4", Project: "p1"},
			want:    &apiv2.IPServiceGetResponse{Ip: &apiv2.IP{Ip: "1.2.3.4", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}},
			wantErr: false,
		},
		{
			name:    "get non existing",
			rq:      &apiv2.IPServiceGetRequest{Ip: "1.2.3.5"},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.Get(ctx, connect.NewRequest(tt.rq))
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
						&apiv2.IP{}, "uuid",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("ipServiceServer.Get() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_ipServiceServer_List(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

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

	repo, container := test.StartRepository(t, log, mdc)
	defer func() {
		_ = container.Terminate(context.Background())
	}()
	ctx := context.Background()

	nws := []*apiv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("internet"), Project: pointer.Pointer("p0"), Prefixes: []string{"1.2.3.0/24"}},
		{Id: pointer.Pointer("internetv6"), Project: pointer.Pointer("p0"), Prefixes: []string{"2001:db8::/96"}},
		{Id: pointer.Pointer("n3"), Project: pointer.Pointer("p2"), Prefixes: []string{"2.3.4.0/24"}},
	}

	ips := []*apiv2.IPServiceCreateRequest{
		{Name: pointer.Pointer("ip1"), Ip: pointer.Pointer("1.2.3.4"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip2"), Ip: pointer.Pointer("1.2.3.5"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip3"), Ip: pointer.Pointer("1.2.3.6"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip4"), Ip: pointer.Pointer("2001:db8::1"), Project: "p2", Network: "internetv6"},
		{Name: pointer.Pointer("ip5"), Ip: pointer.Pointer("2.3.4.5"), Project: "p2", Network: "n3"},
	}

	createNetworks(t, ctx, repo, nws)
	createIPs(t, ctx, repo, ips)

	tests := []struct {
		name           string
		rq             *apiv2.IPServiceListRequest
		want           *apiv2.IPServiceListResponse
		wantReturnCode connect.Code
		wantErr        bool
	}{
		// {
		// 	name:    "get by ip",
		// 	rq:      &apiv2.IPQuery{Ip: pointer.Pointer("1.2.3.4"), Project: pointer.Pointer("p1")},
		// 	want:    &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{{Name: "ip1", Ip: "1.2.3.4", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}}},
		// 	wantErr: false,
		// },
		{
			name:    "get all",
			rq:      &apiv2.IPServiceListRequest{Project: "p1", Query: &apiv2.IPQuery{Project: pointer.Pointer("p1")}},
			want:    &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{{Name: "ip1", Ip: "1.2.3.4", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}}},
			wantErr: false,
		},
		// {
		// 	name: "get by project",
		// 	rq:   &apiv2.IPQuery{Project: pointer.Pointer("p1")},
		// 	want: &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{
		// 		{Name: "ip1", Ip: "1.2.3.4", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
		// 		{Name: "ip2", Ip: "1.2.3.5", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
		// 		{Name: "ip3", Ip: "1.2.3.6", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}}},
		// 	wantErr: false,
		// },
		// {
		// 	name:    "get by addressfamily",
		// 	rq:      &apiv2.IPQuery{AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum(), Project: pointer.Pointer("p2")},
		// 	want:    &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{{Name: "ip4", Ip: "2001:db8::1", Project: "p2", Network: "internetv6", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}}},
		// 	wantErr: false,
		// },
		// {
		// 	name:    "get by parent prefix cidr",
		// 	rq:      &apiv2.IPQuery{ParentPrefixCidr: pointer.Pointer("2.3.4.0/24"), Project: pointer.Pointer("p2")},
		// 	want:    &apiv2.IPServiceListResponse{Ips: []*apiv2.IP{{Name: "ip5", Ip: "2.3.4.5", Project: "p2", Network: "n3", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}}},
		// 	wantErr: false,
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.List(ctx, connect.NewRequest(tt.rq))
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
						&apiv2.IP{}, "uuid",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("ipServiceServer.List() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_ipServiceServer_Update(t *testing.T) {
	log := slog.Default()

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

	repo, container := test.StartRepository(t, log, mdc)

	defer func() {
		_ = container.Terminate(context.Background())
	}()
	ctx := context.Background()

	nws := []*apiv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("internet"), Project: pointer.Pointer("p0"), Prefixes: []string{"1.2.3.0/24"}},
		{Id: pointer.Pointer("internetv6"), Project: pointer.Pointer("p0"), Prefixes: []string{"2001:db8::/96"}},
		{Id: pointer.Pointer("n3"), Project: pointer.Pointer("p2"), Prefixes: []string{"2.3.4.0/24"}},
	}
	ips := []*apiv2.IPServiceCreateRequest{
		{Name: pointer.Pointer("ip1"), Ip: pointer.Pointer("1.2.3.4"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip2"), Ip: pointer.Pointer("1.2.3.5"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip3"), Ip: pointer.Pointer("1.2.3.6"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip4"), Ip: pointer.Pointer("2001:db8::1"), Project: "p2", Network: "internetv6", Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
		{Name: pointer.Pointer("ip5"), Ip: pointer.Pointer("2.3.4.5"), Project: "p2", Network: "n3"},
	}

	createNetworks(t, ctx, repo, nws)
	createIPs(t, ctx, repo, ips)

	tests := []struct {
		name           string
		rq             *apiv2.IPServiceUpdateRequest
		want           *apiv2.IPServiceUpdateResponse
		wantReturnCode connect.Code
		wantErr        bool
	}{
		{
			name:    "update name",
			rq:      &apiv2.IPServiceUpdateRequest{Ip: "1.2.3.4", Project: "p1", Name: pointer.Pointer("ip1-changed")},
			want:    &apiv2.IPServiceUpdateResponse{Ip: &apiv2.IP{Name: "ip1-changed", Ip: "1.2.3.4", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}},
			wantErr: false,
		},
		{
			name:    "update description",
			rq:      &apiv2.IPServiceUpdateRequest{Ip: "1.2.3.5", Project: "p1", Description: pointer.Pointer("test was here")},
			want:    &apiv2.IPServiceUpdateResponse{Ip: &apiv2.IP{Name: "ip2", Ip: "1.2.3.5", Project: "p1", Description: "test was here", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}},
			wantErr: false,
		},
		{
			name:    "update type",
			rq:      &apiv2.IPServiceUpdateRequest{Ip: "1.2.3.6", Project: "p1", Type: apiv2.IPType_IP_TYPE_STATIC.Enum()},
			want:    &apiv2.IPServiceUpdateResponse{Ip: &apiv2.IP{Name: "ip3", Ip: "1.2.3.6", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_STATIC, Meta: &apiv2.Meta{}}},
			wantErr: false,
		},
		{
			name:    "update tags",
			rq:      &apiv2.IPServiceUpdateRequest{Ip: "2001:db8::1", Project: "p2", Labels: &apiv2.Labels{Labels: map[string]string{"color": "red", "purpose": "lb"}}},
			want:    &apiv2.IPServiceUpdateResponse{Ip: &apiv2.IP{Name: "ip4", Ip: "2001:db8::1", Project: "p2", Network: "internetv6", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{Labels: &apiv2.Labels{Labels: map[string]string{"color": "red", "purpose": "lb"}}}}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.Update(ctx, connect.NewRequest(tt.rq))
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
						&apiv2.IP{}, "uuid",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("ipServiceServer.Update() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_ipServiceServer_Delete(t *testing.T) {
	log := slog.Default()

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

	repo, container := test.StartRepository(t, log, mdc)

	defer func() {
		_ = container.Terminate(context.Background())
	}()
	ctx := context.Background()

	nws := []*apiv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("internet"), Project: pointer.Pointer("p0"), Prefixes: []string{"1.2.3.0/24"}},
		{Id: pointer.Pointer("internetv6"), Project: pointer.Pointer("p0"), Prefixes: []string{"2001:db8::/96"}},
		{Id: pointer.Pointer("n3"), Project: pointer.Pointer("p2"), Prefixes: []string{"2.3.4.0/24"}},
	}

	ips := []*apiv2.IPServiceCreateRequest{
		{Name: pointer.Pointer("ip1"), Ip: pointer.Pointer("1.2.3.4"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip2"), Ip: pointer.Pointer("1.2.3.5"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip3"), Ip: pointer.Pointer("1.2.3.6"), Project: "p1", Network: "internet", MachineId: pointer.Pointer("abc")},
		{Name: pointer.Pointer("ip4"), Ip: pointer.Pointer("2001:db8::1"), Project: "p2", Network: "internetv6", Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
		{Name: pointer.Pointer("ip5"), Ip: pointer.Pointer("2.3.4.5"), Project: "p2", Network: "n3"},
	}

	createNetworks(t, ctx, repo, nws)
	createIPs(t, ctx, repo, ips)

	tests := []struct {
		name           string
		rq             *apiv2.IPServiceDeleteRequest
		want           *apiv2.IPServiceDeleteResponse
		wantErr        bool
		wantReturnCode connect.Code
		wantErrMessage string
	}{
		{
			name:    "delete known ip",
			rq:      &apiv2.IPServiceDeleteRequest{Ip: "1.2.3.4", Project: "p1"},
			want:    &apiv2.IPServiceDeleteResponse{Ip: &apiv2.IP{Name: "ip1", Ip: "1.2.3.4", Project: "p1", Network: "internet", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}}},
			wantErr: false,
		},
		{
			name:           "delete unknown ip",
			rq:             &apiv2.IPServiceDeleteRequest{Ip: "1.2.3.7", Project: "p1"},
			want:           nil,
			wantErr:        true,
			wantReturnCode: connect.CodeNotFound,
			wantErrMessage: "not_found: no ip with id \"1.2.3.7\" found",
		},
		{
			name:           "delete machine ip",
			rq:             &apiv2.IPServiceDeleteRequest{Ip: "1.2.3.6", Project: "p1"},
			want:           nil,
			wantErr:        true,
			wantReturnCode: connect.CodeInvalidArgument,
			wantErrMessage: "invalid_argument: ip with machine scope cannot be deleted",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.Delete(ctx, connect.NewRequest(tt.rq))
			if (err != nil) != tt.wantErr {
				t.Errorf("ipServiceServer.Delete() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (err != nil) && tt.wantErr {
				require.Equal(t, tt.wantErrMessage, err.Error())
				var connectErr *connect.Error
				if errors.As(err, &connectErr) && tt.wantReturnCode != connectErr.Code() {
					t.Errorf("ipServiceServer.Delete() errcode = %v, wantReturnCode %v", connectErr.Code(), tt.wantReturnCode)
				}
				return
			}
			if diff := cmp.Diff(
				tt.want, got.Msg,
				cmp.Options{
					protocmp.Transform(),
					protocmp.IgnoreFields(
						&apiv2.IP{}, "uuid",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("ipServiceServer.Delete() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_ipServiceServer_Create(t *testing.T) {
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

	log := slog.Default()
	repo, container := test.StartRepository(t, log, mdc)
	defer func() {
		_ = container.Terminate(context.Background())
	}()
	ctx := context.Background()

	nws := []*apiv2.NetworkServiceCreateRequest{
		{Id: pointer.Pointer("internet"), Prefixes: []string{"1.2.0.0/16"}},
		{Id: pointer.Pointer("tenant-network"), Prefixes: []string{"10.2.0.0/24"}, Options: &apiv2.NetworkOptions{PrivateSuper: true}},
		{Id: pointer.Pointer("tenant-network-v6"), Prefixes: []string{"2001:db8:1::/64"}, Options: &apiv2.NetworkOptions{PrivateSuper: true}},
		{Id: pointer.Pointer("tenant-network-dualstack"), Prefixes: []string{"10.3.0.0/24", "2001:db8:2::/64"}, Options: &apiv2.NetworkOptions{PrivateSuper: true}},
	}
	ips := []*apiv2.IPServiceCreateRequest{
		{Name: pointer.Pointer("ip1"), Ip: pointer.Pointer("1.2.3.4"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip2"), Ip: pointer.Pointer("1.2.3.5"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip3"), Ip: pointer.Pointer("1.2.3.6"), Project: "p1", Network: "internet"},
		{Name: pointer.Pointer("ip4"), Ip: pointer.Pointer("2001:db8:1::1"), Project: "p2", Network: "tenant-network-v6", Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
		{Name: pointer.Pointer("ip5"), Ip: pointer.Pointer("10.2.0.5"), Project: "p2", Network: "tenant-network"},
	}

	createNetworks(t, ctx, repo, nws)
	createIPs(t, ctx, repo, ips)

	tests := []struct {
		name           string
		rq             *apiv2.IPServiceCreateRequest
		want           *apiv2.IPServiceCreateResponse
		wantErr        bool
		wantReturnCode connect.Code
		wantErrMessage string
	}{
		{
			name: "create random ephemeral ipv4",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "1.2.0.1", Network: "internet", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create random ephemeral ipv6",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "tenant-network-v6",
				Project: "p1",
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "2001:db8:1::2", Network: "tenant-network-v6", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create specific ephemeral ipv6",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "tenant-network-v6",
				Project: "p1",
				Ip:      pointer.Pointer("2001:db8:1::99"),
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "2001:db8:1::99", Network: "tenant-network-v6", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create random ephemeral ipv4 from a dualstack network",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "tenant-network-dualstack",
				Project: "p1",
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "10.3.0.1", Network: "tenant-network-dualstack", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create random ephemeral ipv6 from a dualstack network",
			rq: &apiv2.IPServiceCreateRequest{
				Network:       "tenant-network-dualstack",
				Project:       "p1",
				AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum(),
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "2001:db8:2::1", Network: "tenant-network-dualstack", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create specific ephemeral ipv4",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
				Ip:      pointer.Pointer("1.2.0.99"),
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "1.2.0.99", Network: "internet", Project: "p1", Type: apiv2.IPType_IP_TYPE_EPHEMERAL, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create specific static ipv4",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
				Ip:      pointer.Pointer("1.2.0.100"),
				Type:    apiv2.IPType_IP_TYPE_STATIC.Enum(),
			},
			want: &apiv2.IPServiceCreateResponse{
				Ip: &apiv2.IP{Ip: "1.2.0.100", Network: "internet", Project: "p1", Type: apiv2.IPType_IP_TYPE_STATIC, Meta: &apiv2.Meta{}},
			},
		},
		{
			name: "create specific ipv4 which is already allocated",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
				Ip:      pointer.Pointer("1.2.0.1"),
			},
			want:           nil,
			wantErr:        true,
			wantReturnCode: connect.CodeAlreadyExists,
			wantErrMessage: "already_exists: AlreadyAllocatedError: given ip:1.2.0.1 is already allocated", // FIXME potentially a go-ipam error handling bug
		},
		{
			name: "allocate a static specific ip outside prefix",
			rq: &apiv2.IPServiceCreateRequest{
				Network: "internet",
				Project: "p1",
				Ip:      pointer.Pointer("1.3.0.1"),
			},
			want:           nil,
			wantErr:        true,
			wantReturnCode: connect.CodeInvalidArgument,
			wantErrMessage: "invalid_argument: specific ip 1.3.0.1 not contained in any of the defined prefixes", // FIXME potentially a go-ipam error handling bug
		},
		{
			name: "allocate a random ip with unavailable addressfamily",
			rq: &apiv2.IPServiceCreateRequest{
				Network:       "tenant-network-v6",
				Project:       "p1",
				AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.Enum(),
			},
			want:           nil,
			wantErr:        true,
			wantReturnCode: connect.CodeInvalidArgument,
			wantErrMessage: "invalid_argument: there is no prefix for the given addressfamily:IPv4 present in network:tenant-network-v6 [IPv6]",
		},
		{
			name: "allocate a random ip with unavailable addressfamily",
			rq: &apiv2.IPServiceCreateRequest{
				Network:       "tenant-network",
				Project:       "p1",
				AddressFamily: apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.Enum(),
			},
			want:           nil,
			wantErr:        true,
			wantReturnCode: connect.CodeInvalidArgument,
			wantErrMessage: "invalid_argument: there is no prefix for the given addressfamily:IPv6 present in network:tenant-network [IPv4]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &ipServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := i.Create(ctx, connect.NewRequest(tt.rq))
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
						&apiv2.IP{}, "uuid",
					),
					protocmp.IgnoreFields(
						&apiv2.Meta{}, "created_at", "updated_at",
					),
				},
			); diff != "" {
				t.Errorf("ipServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func createIPs(t *testing.T, ctx context.Context, repo *repository.Store, ips []*apiv2.IPServiceCreateRequest) {
	for _, ip := range ips {
		validated, err := repo.IP(nil).ValidateCreate(ctx, ip)
		require.NoError(t, err)

		_, err = repo.IP(nil).Create(ctx, validated)
		require.NoError(t, err)
	}
}

func createNetworks(t *testing.T, ctx context.Context, repo *repository.Store, nws []*apiv2.NetworkServiceCreateRequest) {
	for _, nw := range nws {
		// TODO do not care about project here

		validated, err := repo.Network(nil).ValidateCreate(ctx, nw)
		require.NoError(t, err)
		_, err = repo.Network(nil).Create(ctx, validated)
		require.NoError(t, err)
	}
}
