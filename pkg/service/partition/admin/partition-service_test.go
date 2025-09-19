package admin

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Test_partitionServiceServer_Create(t *testing.T) {
	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.String(), "/invalid") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	invalidURL := ts.URL + "/invalid"
	defer ts.Close()

	tests := []struct {
		name    string
		request *adminv2.PartitionServiceCreateRequest
		want    *adminv2.PartitionServiceCreateResponse
		wantErr error
	}{
		{
			name:    "imageurl is not accessible is nil",
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: invalidURL}}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition imageurl of:partition-1 is not accessible under:%s statuscode:404`, invalidURL),
		},
		{
			name:    "kernelurl is not accessible is nil",
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: invalidURL}}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition kernelurl of:partition-1 is not accessible under:%s statuscode:404`, invalidURL),
		},
		{
			name: "dnsserver is malformed",
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{
				Id:                "partition-1",
				DnsServer:         []*apiv2.DNSServer{{Ip: "1.2.3.4.5"}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`dnsserver ip is not valid:ParseAddr("1.2.3.4.5"): IPv4 address too long`),
		},
		{
			name: "too many dnsserver",
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{
				Id:                "partition-1",
				DnsServer:         []*apiv2.DNSServer{{Ip: "1.2.3.4"}, {Ip: "1.2.3.5"}, {Ip: "1.2.3.6"}, {Ip: "1.2.3.7"}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`not more than 3 dnsservers must be specified`),
		},
		{
			name: "ntpserver is malformed",
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{
				Id:                "partition-1",
				DnsServer:         []*apiv2.DNSServer{{Ip: "1.2.3.4"}},
				NtpServer:         []*apiv2.NTPServer{{Address: "1:3"}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`dns name: 1:3 for ntp server not correct`),
		},
		{
			name:    "valid partition",
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			want: &adminv2.PartitionServiceCreateResponse{
				Partition: &apiv2.Partition{Id: "partition-1", Meta: &apiv2.Meta{}, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			},
			wantErr: nil,
		},
		{
			name:    "partition already exists",
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			want:    nil,
			wantErr: errorutil.Conflict("cannot create partition in database, entity already exists: partition-1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &partitionServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := p.Create(ctx, connect.NewRequest(tt.request))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "meta", "expires_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("partitionServiceServer.Create() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_partitionServiceServer_Update(t *testing.T) {
	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()
	repo := testStore.Store

	ctx := t.Context()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.String(), "/invalid") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	invalidURL := ts.URL + "/invalid"
	defer ts.Close()

	partitionMap := test.CreatePartitions(t, repo, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
		{
			Partition: &apiv2.Partition{Id: "partition-2", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
		{
			Partition: &apiv2.Partition{Id: "partition-3", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
		{
			Partition: &apiv2.Partition{Id: "partition-4", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
	})

	tests := []struct {
		name    string
		request *adminv2.PartitionServiceUpdateRequest
		want    *adminv2.PartitionServiceUpdateResponse
		wantErr error
	}{
		{
			name:    "imageurl is not accessible is nil",
			request: &adminv2.PartitionServiceUpdateRequest{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: invalidURL}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition imageurl of:partition-1 is not accessible under:%s statuscode:404`, invalidURL),
		},
		{
			name:    "kernelurl is not accessible is nil",
			request: &adminv2.PartitionServiceUpdateRequest{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: invalidURL}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition kernelurl of:partition-1 is not accessible under:%s statuscode:404`, invalidURL),
		},
		{
			name: "dnsserver is malformed",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id:                "partition-1",
				DnsServer:         []*apiv2.DNSServer{{Ip: "1.2.3.4.5"}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`dnsserver ip is not valid:ParseAddr("1.2.3.4.5"): IPv4 address too long`),
		},
		{
			name: "too many dnsserver",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id:                "partition-1",
				DnsServer:         []*apiv2.DNSServer{{Ip: "1.2.3.4"}, {Ip: "1.2.3.5"}, {Ip: "1.2.3.6"}, {Ip: "1.2.3.7"}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`not more than 3 dnsservers must be specified`),
		},
		{
			name: "ntpserver is malformed",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id:                "partition-1",
				DnsServer:         []*apiv2.DNSServer{{Ip: "1.2.3.4"}},
				NtpServer:         []*apiv2.NTPServer{{Address: "1:3"}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`dns name: 1:3 for ntp server not correct`),
		},
		{
			name: "valid partition, change nothing",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id: "partition-1",
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(partitionMap["partition-1"].Changed),
				},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			want: &adminv2.PartitionServiceUpdateResponse{
				Partition: &apiv2.Partition{
					Id:                "partition-1",
					Meta:              &apiv2.Meta{Generation: 1},
					BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			},
			wantErr: nil,
		},
		{
			name: "valid partition, change image url",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id: "partition-2",
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(partitionMap["partition-2"].Changed),
				},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL + "/changed", KernelUrl: validURL}},
			want: &adminv2.PartitionServiceUpdateResponse{
				Partition: &apiv2.Partition{
					Id:                "partition-2",
					Meta:              &apiv2.Meta{Generation: 1},
					BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL + "/changed", KernelUrl: validURL}},
			},
			wantErr: nil,
		},
		{
			name: "valid partition, add labels",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id: "partition-3",
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(partitionMap["partition-3"].Changed),
				},
				Labels:            &apiv2.UpdateLabels{Update: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL + "/changed", KernelUrl: validURL}},
			want: &adminv2.PartitionServiceUpdateResponse{
				Partition: &apiv2.Partition{
					Id:                "partition-3",
					Meta:              &apiv2.Meta{Labels: &apiv2.Labels{Labels: map[string]string{"color": "red"}}, Generation: 1},
					BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL + "/changed", KernelUrl: validURL}},
			},
			wantErr: nil,
		},
		{
			name: "client side optimistic lock handling fails with wrong timestamp from the past",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id: "partition-4",
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(time.Date(2002, 2, 12, 12, 0, 0, 0, time.UTC)),
				},
				Description: pointer.Pointer(""),
			},
			wantErr: errorutil.Conflict(`cannot update partition (partition-4): the entity was already modified, please retry`),
		},
		{
			name: "client side optimistic lock handling fails with wrong timestamp from the future",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id: "partition-4",
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(time.Now().Add(10 * time.Minute)),
				},
				Description: pointer.Pointer(""),
			},
			wantErr: errorutil.Conflict(`cannot update partition (partition-4): the entity was already modified, please retry`),
		},
		{
			name: "client side optimistic lock handling fails with empty timestamp (should be prevented by protovalidate, but for completeness...)",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id:          "partition-4",
				UpdateMeta:  nil,
				Description: pointer.Pointer(""),
			},
			wantErr: errorutil.InvalidArgument(`update meta must be set`),
		},
		{
			name: "server side optimistic lock handling succeeds",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id: "partition-4",
				UpdateMeta: &apiv2.UpdateMeta{
					LockingStrategy: apiv2.OptimisticLockingStrategy_OPTIMISTIC_LOCKING_STRATEGY_SERVER,
				},
				Description: pointer.Pointer(""),
			},
			wantErr: nil,
			want: &adminv2.PartitionServiceUpdateResponse{
				Partition: &apiv2.Partition{
					Id:                "partition-4",
					Meta:              &apiv2.Meta{Generation: 1},
					BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL},
					Description:       "",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &partitionServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := p.Update(ctx, connect.NewRequest(tt.request))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "expires_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("partitionServiceServer.Update() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}

func Test_partitionServiceServer_Delete(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
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
		{Partition: &apiv2.Partition{Id: "partition-1", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: "partition-2", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: "partition-3", BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	test.CreateNetworks(t, repo, []*adminv2.NetworkServiceCreateRequest{
		{
			Id:                       pointer.Pointer("tenant-super-network"),
			Prefixes:                 []string{"10.100.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: pointer.Pointer(uint32(22))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                pointer.Pointer("partition-2"),
		},
	})

	test.CreateMachines(t, testStore, []*metal.Machine{
		{Base: metal.Base{ID: "m1"}, PartitionID: "partition-3", SizeID: "c1-large-x86"},
	})

	tests := []struct {
		name    string
		request *adminv2.PartitionServiceDeleteRequest
		want    *adminv2.PartitionServiceDeleteResponse
		wantErr error
	}{
		{
			name:    "delete non existing",
			request: &adminv2.PartitionServiceDeleteRequest{Id: "partition-4"},
			want:    nil,
			wantErr: errorutil.NotFound(`no partition with id "partition-4" found`),
		},
		{
			name:    "delete with attached network",
			request: &adminv2.PartitionServiceDeleteRequest{Id: "partition-2"},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`there are still networks in "partition-2"`),
		},
		{
			name:    "delete with a machine",
			request: &adminv2.PartitionServiceDeleteRequest{Id: "partition-3"},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`there are still machines in "partition-3"`),
		},
		{
			name:    "delete existing",
			request: &adminv2.PartitionServiceDeleteRequest{Id: "partition-1"},
			wantErr: nil,
			want: &adminv2.PartitionServiceDeleteResponse{
				Partition: &apiv2.Partition{Id: "partition-1", Meta: &apiv2.Meta{}, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &partitionServiceServer{
				log:  log,
				repo: repo,
			}
			got, err := p.Delete(ctx, connect.NewRequest(tt.request))
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, pointer.SafeDeref(got).Msg,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "meta", "expires_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("partitionServiceServer.Delete() = %v, want %vņdiff: %s", got.Msg, tt.want, diff)
			}
		})
	}
}
