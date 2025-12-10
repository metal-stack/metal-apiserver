package test

import (
	"context"
	"log/slog"
	"net"
	"testing"

	"github.com/cockroachdb/cockroach-go/v2/testserver"
	"github.com/jmoiron/sqlx"
	apiv1 "github.com/metal-stack/masterdata-api/api/v1"
	mdc "github.com/metal-stack/masterdata-api/pkg/client"
	"github.com/metal-stack/masterdata-api/pkg/datastore"
	"github.com/metal-stack/masterdata-api/pkg/service"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func StartMasterdataWithPostgres(t testing.TB, log *slog.Logger) (mdc.Client, *grpc.ClientConn, func()) {
	ctx := context.Background()

	postgres, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithPassword("password"),
		postgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)

	connectionString, err := postgres.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sqlx.Open("postgres", connectionString)
	require.NoError(t, err)

	closer := func() {
		_ = postgres.Terminate(ctx)
	}

	return startMasterdataWithDB(t, log, closer, db)
}

func StartMasterdataWithCockroach(t testing.TB, log *slog.Logger) (mdc.Client, *grpc.ClientConn, func()) {
	cr, err := testserver.NewTestServer()
	require.NoError(t, err)

	db, err := sqlx.Open("postgres", cr.PGURL().String())
	require.NoError(t, err)
	closer := func() {
		cr.Stop()
	}
	return startMasterdataWithDB(t, log, closer, db)
}

func startMasterdataWithDB(t testing.TB, log *slog.Logger, dbcloser func(), db *sqlx.DB) (mdc.Client, *grpc.ClientConn, func()) {

	log = log.WithGroup("masterdata")
	ps := datastore.New(log, db, &apiv1.Project{})
	pms := datastore.New(log, db, &apiv1.ProjectMember{})
	ts := datastore.New(log, db, &apiv1.Tenant{})
	tms := datastore.New(log, db, &apiv1.TenantMember{})

	err := datastore.InitTables(log, db,
		&apiv1.Project{},
		&apiv1.ProjectMember{},
		&apiv1.Tenant{},
		&apiv1.TenantMember{},
	)
	require.NoError(t, err)

	projectService := service.NewProjectService(log, ps, pms, ts)
	projectMemberService := service.NewProjectMemberService(log, ps, pms, ts)
	tenantService := service.NewTenantService(db, log, ts, tms)
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
		mdc.NamespaceInterceptor("metal-stack.io"),
	)
	require.NoError(t, err)

	mc := &memoryClient{conn: conn}

	closer := func() {
		dbcloser()
		grpcServer.Stop()
	}

	return mc, conn, closer
}

func StartMasterdataInMemory(t testing.TB, log *slog.Logger) (mdc.Client, *grpc.ClientConn, func()) {
	log = log.WithGroup("masterdata")

	ps := datastore.NewMemory(log, &apiv1.Project{})
	pms := datastore.NewMemory(log, &apiv1.ProjectMember{})
	ts := datastore.NewMemory(log, &apiv1.Tenant{})
	tms := datastore.NewMemory(log, &apiv1.TenantMember{})

	projectService := service.NewProjectService(log, ps, pms, ts)
	projectMemberService := service.NewProjectMemberService(log, ps, pms, ts)
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
		mdc.NamespaceInterceptor("metal-stack.io"),
	)
	require.NoError(t, err)

	mc := &memoryClient{conn: conn}

	closer := func() {
		grpcServer.Stop()
	}

	return mc, conn, closer
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

func (c memoryClient) Version() apiv1.VersionServiceClient {
	return apiv1.NewVersionServiceClient(c.conn)
}
