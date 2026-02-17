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

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	partition1 = "partition-1"
	partition2 = "partition-2"
	partition3 = "partition-3"
	partition4 = "partition-4"

	p1 = "00000000-0000-0000-0000-000000000001"
	p2 = "00000000-0000-0000-0000-000000000002"
)

func Test_partitionServiceServer_Create(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

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
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: invalidURL}}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition imageurl of:partition-1 is not accessible under:%s statuscode:404`, invalidURL),
		},
		{
			name:    "kernelurl is not accessible is nil",
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: invalidURL}}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition kernelurl of:partition-1 is not accessible under:%s statuscode:404`, invalidURL),
		},
		{
			name: "dnsserver is malformed",
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{
				Id:                partition1,
				DnsServer:         []*apiv2.DNSServer{{Ip: "1.2.3.4.5"}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`dnsserver ip is not valid:ParseAddr("1.2.3.4.5"): IPv4 address too long`),
		},
		{
			name: "too many dnsserver",
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{
				Id:                partition1,
				DnsServer:         []*apiv2.DNSServer{{Ip: "1.2.3.4"}, {Ip: "1.2.3.5"}, {Ip: "1.2.3.6"}, {Ip: "1.2.3.7"}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`not more than 3 dnsservers must be specified`),
		},
		{
			name: "ntpserver is malformed",
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{
				Id:                partition1,
				DnsServer:         []*apiv2.DNSServer{{Ip: "1.2.3.4"}},
				NtpServer:         []*apiv2.NTPServer{{Address: "1:3"}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`dns name: 1:3 for ntp server not correct`),
		},
		{
			name:    "valid partition",
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			want: &adminv2.PartitionServiceCreateResponse{
				Partition: &apiv2.Partition{Id: partition1, Meta: &apiv2.Meta{}, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			},
			wantErr: nil,
		},
		{
			name:    "partition already exists",
			request: &adminv2.PartitionServiceCreateRequest{Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
			want:    nil,
			wantErr: errorutil.Conflict("cannot create partition in database, entity already exists: partition-1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &partitionServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.request)
			}
			got, err := p.Create(ctx, tt.request)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "meta", "expires_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("partitionServiceServer.Create() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_partitionServiceServer_Update(t *testing.T) {
	t.Parallel()

	log := slog.Default()

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

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

	partitionMap := test.CreatePartitions(t, testStore, []*adminv2.PartitionServiceCreateRequest{
		{
			Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
		{
			Partition: &apiv2.Partition{Id: partition2, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
		},
		{
			Partition: &apiv2.Partition{Id: partition3, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
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
			request: &adminv2.PartitionServiceUpdateRequest{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: invalidURL}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition imageurl of:partition-1 is not accessible under:%s statuscode:404`, invalidURL),
		},
		{
			name:    "kernelurl is not accessible is nil",
			request: &adminv2.PartitionServiceUpdateRequest{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: invalidURL}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`partition kernelurl of:partition-1 is not accessible under:%s statuscode:404`, invalidURL),
		},
		{
			name: "dnsserver is malformed",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id:                partition1,
				DnsServer:         []*apiv2.DNSServer{{Ip: "1.2.3.4.5"}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`dnsserver ip is not valid:ParseAddr("1.2.3.4.5"): IPv4 address too long`),
		},
		{
			name: "too many dnsserver",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id:                partition1,
				DnsServer:         []*apiv2.DNSServer{{Ip: "1.2.3.4"}, {Ip: "1.2.3.5"}, {Ip: "1.2.3.6"}, {Ip: "1.2.3.7"}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`not more than 3 dnsservers must be specified`),
		},
		{
			name: "ntpserver is malformed",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id:                partition1,
				DnsServer:         []*apiv2.DNSServer{{Ip: "1.2.3.4"}},
				NtpServer:         []*apiv2.NTPServer{{Address: "1:3"}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`dns name: 1:3 for ntp server not correct`),
		},
		{
			name: "valid partition, change nothing",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id: partition1,
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(partitionMap[partition1].Meta.UpdatedAt.AsTime()),
				},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			want: &adminv2.PartitionServiceUpdateResponse{
				Partition: &apiv2.Partition{
					Id:                partition1,
					Meta:              &apiv2.Meta{Generation: 1},
					BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			},
			wantErr: nil,
		},
		{
			name: "valid partition, change image url",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id: partition2,
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(partitionMap[partition2].Meta.UpdatedAt.AsTime()),
				},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL + "/changed", KernelUrl: validURL}},
			want: &adminv2.PartitionServiceUpdateResponse{
				Partition: &apiv2.Partition{
					Id:                partition2,
					Meta:              &apiv2.Meta{Generation: 1},
					BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL + "/changed", KernelUrl: validURL}},
			},
			wantErr: nil,
		},
		{
			name: "valid partition, add labels",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id: partition3,
				UpdateMeta: &apiv2.UpdateMeta{
					UpdatedAt: timestamppb.New(partitionMap[partition3].Meta.UpdatedAt.AsTime()),
				},
				Labels:            &apiv2.UpdateLabels{Update: &apiv2.Labels{Labels: map[string]string{"color": "red"}}},
				BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL + "/changed", KernelUrl: validURL}},
			want: &adminv2.PartitionServiceUpdateResponse{
				Partition: &apiv2.Partition{
					Id:                partition3,
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
				Description: new(""),
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
				Description: new(""),
			},
			wantErr: errorutil.Conflict(`cannot update partition (partition-4): the entity was already modified, please retry`),
		},
		{
			name: "client side optimistic lock handling fails with empty timestamp (should be prevented by protovalidate, but for completeness...)",
			request: &adminv2.PartitionServiceUpdateRequest{
				Id:          "partition-4",
				UpdateMeta:  nil,
				Description: new(""),
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
				Description: new(""),
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
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.request)
			}
			got, err := p.Update(ctx, tt.request)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "expires_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("partitionServiceServer.Update() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_partitionServiceServer_Delete(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	testStore, closer := test.StartRepositoryWithCleanup(t, log)
	defer closer()

	ctx := t.Context()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "a image")
	}))

	validURL := ts.URL
	defer ts.Close()

	test.CreatePartitions(t, testStore, []*adminv2.PartitionServiceCreateRequest{
		{Partition: &apiv2.Partition{Id: partition1, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: partition2, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: partition3, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
		{Partition: &apiv2.Partition{Id: partition4, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}}},
	})

	test.CreateNetworks(t, testStore, []*adminv2.NetworkServiceCreateRequest{
		{
			Id:                       new("tenant-super-network"),
			Prefixes:                 []string{"10.100.0.0/14"},
			DefaultChildPrefixLength: &apiv2.ChildPrefixLength{Ipv4: new(uint32(22))},
			Type:                     apiv2.NetworkType_NETWORK_TYPE_SUPER,
			Partition:                new(partition2),
		},
	})

	test.CreateMachines(t, testStore, []*metal.Machine{
		{Base: metal.Base{ID: "m1"}, PartitionID: partition3, SizeID: "c1-large-x86"},
	})

	test.CreateTenants(t, testStore, []*apiv2.TenantServiceCreateRequest{{Name: "t1"}})
	test.CreateProjects(t, testStore, []*apiv2.ProjectServiceCreateRequest{{Name: p1, Login: "t1"}, {Name: p2, Login: "t1"}})

	sizes := []*adminv2.SizeServiceCreateRequest{
		{Size: &apiv2.Size{
			Id: "n1-medium-x86", Name: new("n1-medium-x86"),
			Constraints: []*apiv2.SizeConstraint{
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 4, Max: 4},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1024 * 1024, Max: 1024 * 1024},
				{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 10 * 1024 * 1024, Max: 10 * 1024 * 1024},
			},
		}},
	}
	sizeReservations := []*adminv2.SizeReservationServiceCreateRequest{
		{SizeReservation: &apiv2.SizeReservation{
			Name:        "sz-n1",
			Description: "N1 Reservation for project-1 in partition-4",
			Project:     p1,
			Size:        "n1-medium-x86",
			Partitions:  []string{partition4},
			Amount:      2,
		}},
	}
	test.CreateSizes(t, testStore, sizes)
	test.CreateSizeReservations(t, testStore, sizeReservations)

	tests := []struct {
		name    string
		request *adminv2.PartitionServiceDeleteRequest
		want    *adminv2.PartitionServiceDeleteResponse
		wantErr error
	}{
		{
			name:    "delete non existing",
			request: &adminv2.PartitionServiceDeleteRequest{Id: "partition-5"},
			want:    nil,
			wantErr: errorutil.NotFound(`no partition with id "partition-5" found`),
		},
		{
			name:    "delete with attached network",
			request: &adminv2.PartitionServiceDeleteRequest{Id: partition2},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`there are still networks in "partition-2"`),
		},
		{
			name:    "delete with a machine",
			request: &adminv2.PartitionServiceDeleteRequest{Id: partition3},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`there are still machines in "partition-3"`),
		},
		{
			name:    "delete with a size reservation",
			request: &adminv2.PartitionServiceDeleteRequest{Id: partition4},
			want:    nil,
			wantErr: errorutil.InvalidArgument(`there are still size reservations in "partition-4"`),
		},
		{
			name:    "delete existing",
			request: &adminv2.PartitionServiceDeleteRequest{Id: partition1},
			wantErr: nil,
			want: &adminv2.PartitionServiceDeleteResponse{
				Partition: &apiv2.Partition{Id: partition1, Meta: &apiv2.Meta{}, BootConfiguration: &apiv2.PartitionBootConfiguration{ImageUrl: validURL, KernelUrl: validURL}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &partitionServiceServer{
				log:  log,
				repo: testStore.Store,
			}
			if tt.wantErr == nil {
				// Execute proto based validation
				test.Validate(t, tt.request)
			}
			got, err := p.Delete(ctx, tt.request)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Image{}, "meta", "expires_at",
				),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("partitionServiceServer.Delete() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}

func Test_partitionServiceServer_Capacity(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := t.Context()

	dc := test.NewDatacenter(t, log)
	defer dc.Close()

	tests := []struct {
		name    string
		request *adminv2.PartitionServiceCapacityRequest
		before  func()
		want    *adminv2.PartitionServiceCapacityResponse
		wantErr error
	}{
		{
			name:    "one allocated machine",
			request: &adminv2.PartitionServiceCapacityRequest{Id: &partition1},
			before: func() {
				dc.CleanUp(t)
				dc.Create(&scenarios.DefaultDatacenter)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeC1Large, PhonedHome: 1, Allocated: 1, Total: 1},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name:    "two allocated machine",
			request: &adminv2.PartitionServiceCapacityRequest{Id: &partition1},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeC1Large, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine2, scenarios.Partition1, scenarios.SizeC1Large, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
				}

				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeC1Large, PhonedHome: 2, Allocated: 2, Total: 2},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name:    "one faulty, allocated machine",
			request: &adminv2.PartitionServiceCapacityRequest{Id: &partition1},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Machines[0].Machine.IPMI.Address = ""

				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeC1Large, PhonedHome: 1, Allocated: 1, Total: 1, Faulty: 1, FaultyMachines: []string{scenarios.Machine1}},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name: "filter considers all machines",
			request: &adminv2.PartitionServiceCapacityRequest{
				Id:   &partition1,
				Size: new(scenarios.SizeC1Large),
			},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeC1Large, scenarios.Tenant1Project1, metal.MachineLivelinessDead),
					scenarios.MachineFunc(scenarios.Machine2, scenarios.Partition1, scenarios.SizeC1Large, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine3, scenarios.Partition1, scenarios.SizeC1Large, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine4, scenarios.Partition1, scenarios.SizeN1Medium, scenarios.Tenant1Project1, metal.MachineLivelinessDead),
					scenarios.MachineFunc(scenarios.Machine5, scenarios.Partition2, scenarios.SizeC1Large, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
				}

				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeC1Large, PhonedHome: 3, Allocated: 3, Total: 3, Faulty: 1, FaultyMachines: []string{scenarios.Machine1}},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name:    "non filter considers all machines",
			request: &adminv2.PartitionServiceCapacityRequest{},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Partitions = []string{scenarios.Partition1, scenarios.Partition2}
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeC1Large, scenarios.Tenant1Project1, metal.MachineLivelinessDead),
					scenarios.MachineFunc(scenarios.Machine2, scenarios.Partition1, scenarios.SizeC1Large, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine3, scenarios.Partition1, scenarios.SizeC1Large, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine4, scenarios.Partition1, scenarios.SizeN1Medium, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine5, scenarios.Partition2, scenarios.SizeC1Large, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
				}
				testDC.Machines[3].Machine.IPMI.Address = ""
				testDC.SizeReservations = []*adminv2.SizeReservationServiceCreateRequest{}

				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeC1Large, PhonedHome: 3, Allocated: 3, Total: 3, Faulty: 1, FaultyMachines: []string{scenarios.Machine1}},
						{Size: scenarios.SizeN1Medium, PhonedHome: 1, Allocated: 1, Total: 1, Faulty: 1, FaultyMachines: []string{scenarios.Machine4}},
					},
				},
				{
					Partition: partition2,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeC1Large, PhonedHome: 1, Allocated: 1, Total: 1, Faulty: 0},
					},
				}},
			},
			wantErr: nil,
		},
		{
			name:    "one waiting machine",
			request: &adminv2.PartitionServiceCapacityRequest{Id: &partition1},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeC1Large, "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[0].Machine.Waiting = true
				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeC1Large, Waiting: 1, Free: 1, Total: 1, Allocatable: 1},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name:    "one dead machine",
			request: &adminv2.PartitionServiceCapacityRequest{Id: &partition1},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeC1Large, "", metal.MachineLivelinessDead),
				}
				testDC.Machines[0].Machine.Waiting = true
				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeC1Large, Waiting: 1, Faulty: 1, Total: 1, Unavailable: 1, FaultyMachines: []string{scenarios.Machine1}},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name:    "one waiting, one allocated machine",
			request: &adminv2.PartitionServiceCapacityRequest{Id: &partition1},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeC1Large, "", metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine2, scenarios.Partition1, scenarios.SizeC1Large, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
				}
				testDC.Machines[0].Machine.Waiting = true
				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeC1Large, Waiting: 1, Allocated: 1, Total: 2, PhonedHome: 1, Free: 1, Allocatable: 1},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name:    "one free machine",
			request: &adminv2.PartitionServiceCapacityRequest{Id: &partition1},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeC1Large, "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[0].Machine.Waiting = true
				testDC.Machines[0].Machine.State.Value = metal.AvailableState
				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeC1Large, Waiting: 1, Total: 1, Free: 1, Allocatable: 1},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name:    "one machine rebooting",
			request: &adminv2.PartitionServiceCapacityRequest{Id: &partition1},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeC1Large, "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[0].Machine.Waiting = false
				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeC1Large, Total: 1, Other: 1, Unavailable: 1, OtherMachines: []string{scenarios.Machine1}},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name:    "reserved machine does not count as free",
			request: &adminv2.PartitionServiceCapacityRequest{Id: &partition1},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeN1Medium, "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[0].Machine.Waiting = true
				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeN1Medium, Total: 1, Waiting: 1, Free: 0, Allocatable: 1, Reservations: 1, UsedReservations: 0, RemainingReservations: 1},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name:    "overbooked partition, free count capped at 0",
			request: &adminv2.PartitionServiceCapacityRequest{Id: &partition1},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.ProjectsPerTenant = 2
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeN1Medium, "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[0].Machine.Waiting = true
				testDC.SizeReservations = []*adminv2.SizeReservationServiceCreateRequest{
					{
						SizeReservation: &apiv2.SizeReservation{
							Name:        "sz-n1-p1",
							Description: "N1 Reservation for project-1 in partition-1",
							Project:     scenarios.Tenant1Project1,
							Size:        scenarios.SizeN1Medium,
							Partitions:  []string{scenarios.Partition1},
							Amount:      1,
						},
					},
					{
						SizeReservation: &apiv2.SizeReservation{
							Name:        "sz-n1-p2",
							Description: "N1 Reservation for project-2 in partition-1",
							Project:     scenarios.Tenant1Project2,
							Size:        scenarios.SizeN1Medium,
							Partitions:  []string{scenarios.Partition1},
							Amount:      2,
						},
					},
				}
				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeN1Medium, Total: 1, Waiting: 1, Free: 0, Allocatable: 1, Reservations: 3, UsedReservations: 0, RemainingReservations: 3},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name:    "reservations already used up (edge)",
			request: &adminv2.PartitionServiceCapacityRequest{Id: &partition1},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeN1Medium, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine2, scenarios.Partition1, scenarios.SizeN1Medium, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine3, scenarios.Partition1, scenarios.SizeN1Medium, "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[2].Machine.Waiting = true
				testDC.SizeReservations = []*adminv2.SizeReservationServiceCreateRequest{
					{
						SizeReservation: &apiv2.SizeReservation{
							Name:        "sz-n1-p1",
							Description: "N1 Reservation for project-1 in partition-1",
							Project:     scenarios.Tenant1Project1,
							Size:        scenarios.SizeN1Medium,
							Partitions:  []string{scenarios.Partition1},
							Amount:      2,
						},
					},
				}
				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeN1Medium, Total: 3, Waiting: 1, Free: 1, Allocatable: 1, Allocated: 2, Reservations: 2, UsedReservations: 2, PhonedHome: 2},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name:    "reservations already used up",
			request: &adminv2.PartitionServiceCapacityRequest{Id: &partition1},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeN1Medium, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine2, scenarios.Partition1, scenarios.SizeN1Medium, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine3, scenarios.Partition1, scenarios.SizeN1Medium, "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[2].Machine.Waiting = true
				testDC.SizeReservations = []*adminv2.SizeReservationServiceCreateRequest{
					{
						SizeReservation: &apiv2.SizeReservation{
							Name:        "sz-n1-p1",
							Description: "N1 Reservation for project-1 in partition-1",
							Project:     scenarios.Tenant1Project1,
							Size:        scenarios.SizeN1Medium,
							Partitions:  []string{scenarios.Partition1},
							Amount:      1,
						},
					},
				}
				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeN1Medium, Total: 3, Waiting: 1, Free: 1, Allocatable: 1, Allocated: 2, Reservations: 1, UsedReservations: 1, PhonedHome: 2},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name:    "other partition size reservation has no influence",
			request: &adminv2.PartitionServiceCapacityRequest{Id: &partition1},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Partitions = []string{scenarios.Partition1, scenarios.Partition2}
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeN1Medium, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine2, scenarios.Partition1, scenarios.SizeN1Medium, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine3, scenarios.Partition1, scenarios.SizeN1Medium, "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[2].Machine.Waiting = true
				testDC.SizeReservations = []*adminv2.SizeReservationServiceCreateRequest{
					{
						SizeReservation: &apiv2.SizeReservation{
							Name:        "sz-n1-p1",
							Description: "N1 Reservation for project-1 in partition-1",
							Project:     scenarios.Tenant1Project1,
							Size:        scenarios.SizeN1Medium,
							Partitions:  []string{scenarios.Partition1},
							Amount:      2,
						},
					},
					{
						SizeReservation: &apiv2.SizeReservation{
							Name:        "sz-n1-p1",
							Description: "N1 Reservation for project-1 in partition-2",
							Project:     scenarios.Tenant1Project1,
							Size:        scenarios.SizeN1Medium,
							Partitions:  []string{scenarios.Partition2},
							Amount:      2,
						},
					},
				}
				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeN1Medium, Total: 3, Waiting: 1, Free: 1, Allocatable: 1, Allocated: 2, Reservations: 2, UsedReservations: 2, PhonedHome: 2},
					},
				},
			}},
			wantErr: nil,
		},
		{
			name: "evaluate capacity for specific project",
			request: &adminv2.PartitionServiceCapacityRequest{
				Id:      &partition1,
				Project: new(scenarios.Tenant1Project1),
			},
			before: func() {
				dc.CleanUp(t)
				testDC := scenarios.DefaultDatacenter
				testDC.Partitions = []string{scenarios.Partition1, scenarios.Partition2}
				testDC.Machines = []*scenarios.MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]{
					scenarios.MachineFunc(scenarios.Machine1, scenarios.Partition1, scenarios.SizeN1Medium, scenarios.Tenant1Project1, metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine2, scenarios.Partition1, scenarios.SizeN1Medium, "", metal.MachineLivelinessAlive),
					scenarios.MachineFunc(scenarios.Machine3, scenarios.Partition1, scenarios.SizeN1Medium, "", metal.MachineLivelinessAlive),
				}
				testDC.Machines[1].Machine.Waiting = true
				testDC.Machines[2].Machine.Waiting = true
				testDC.SizeReservations = []*adminv2.SizeReservationServiceCreateRequest{
					{
						SizeReservation: &apiv2.SizeReservation{
							Name:        "sz-n1-p1",
							Description: "N1 Reservation for project-1 in partition-1",
							Project:     scenarios.Tenant1Project1,
							Size:        scenarios.SizeN1Medium,
							Partitions:  []string{scenarios.Partition1},
							Amount:      3,
						},
					},
				}
				dc.Create(&testDC)
			},
			want: &adminv2.PartitionServiceCapacityResponse{PartitionCapacity: []*adminv2.PartitionCapacity{
				{
					Partition: partition1,
					MachineSizeCapacities: []*adminv2.MachineSizeCapacity{
						{Size: scenarios.SizeN1Medium, Total: 3, Waiting: 2, Free: 2, Allocatable: 2, Allocated: 1, Reservations: 3, UsedReservations: 1, RemainingReservations: 2, PhonedHome: 1},
					},
				},
			}},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.before()
			p := &partitionServiceServer{
				log:  log,
				repo: dc.TestStore.Store,
			}
			if tt.wantErr == nil {
				test.Validate(t, tt.request)
			}
			got, err := p.Capacity(ctx, tt.request)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if diff := cmp.Diff(
				tt.want, got,
				protocmp.Transform(),
				protocmp.IgnoreFields(
					&apiv2.Meta{}, "created_at", "updated_at",
				),
			); diff != "" {
				t.Errorf("partitionServiceServer.Capacity() = %v, want %vņdiff: %s", got, tt.want, diff)
			}
		})
	}
}
