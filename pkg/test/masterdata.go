package test

import (
	"context"
	"log/slog"
	"net"
	"testing"

	apiv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdc "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/masterdata-api/pkg/datastore"
	"github.com/metal-stack/masterdata-api/pkg/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func StartMasterdataInMemory(t *testing.T, log *slog.Logger) (mdc.Client, *grpc.ClientConn) {
	ps := datastore.NewMemory(log, &apiv1.Project{})
	pms := datastore.NewMemory(log, &apiv1.ProjectMember{})
	ts := datastore.NewMemory(log, &apiv1.Tenant{})
	tms := datastore.NewMemory(log, &apiv1.TenantMember{})

	projectService := service.NewProjectService(log, ps, pms, ts)
	projectMemberService := service.NewProjectMemberService(log, ps, pms, ts)
	// FIXME db should not be required here
	tenantService := service.NewTenantService(nil, log, ts, tms)

	tenantMemberService := service.NewTenantMemberService(log, ts, tms)

	grpcServer := grpc.NewServer()

	apiv1.RegisterProjectServiceServer(grpcServer, projectService)
	apiv1.RegisterProjectMemberServiceServer(grpcServer, projectMemberService)
	apiv1.RegisterTenantServiceServer(grpcServer, tenantService)
	apiv1.RegisterTenantMemberServiceServer(grpcServer, tenantMemberService)

	var (
		buffer = 101024 * 1024
		lis    = bufconn.Listen(buffer)
	)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Error("unable to start grpc server", "error", err)
		}
	}()

	dialer := &dialer{
		lis: lis,
	}

	conn, err := grpc.NewClient(
		// See: https://github.com/grpc/grpc-go/issues/7091 why passthrough is required here
		"passthrough:///bufnet",
		grpc.WithContextDialer(dialer.bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("error connecting to grpc server:%v", err)
	}

	mc := &memoryClient{conn: conn}

	return mc, conn
}

type dialer struct {
	lis *bufconn.Listener
}

func (d *dialer) bufDialer(ctx context.Context, address string) (net.Conn, error) {
	return d.lis.Dial()
}

type memoryClient struct {
	conn *grpc.ClientConn
}

// Close the underlying connection
func (c memoryClient) Close() error {
	return c.conn.Close()
}

// Project is the root accessor for project related functions
func (c memoryClient) Project() apiv1.ProjectServiceClient {
	return apiv1.NewProjectServiceClient(c.conn)
}

// ProjectMember is the root accessor for project member related functions
func (c memoryClient) ProjectMember() apiv1.ProjectMemberServiceClient {
	return apiv1.NewProjectMemberServiceClient(c.conn)
}

// Tenant is the root accessor for tenant related functions
func (c memoryClient) Tenant() apiv1.TenantServiceClient {
	return apiv1.NewTenantServiceClient(c.conn)
}

// Tenant is the root accessor for tenant related functions
func (c memoryClient) TenantMember() apiv1.TenantMemberServiceClient {
	return apiv1.NewTenantMemberServiceClient(c.conn)
}
