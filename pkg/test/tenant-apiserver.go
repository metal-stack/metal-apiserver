package test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmoiron/sqlx"
	apiv1 "github.com/metal-stack/tenant-api/go/api/v1"
	"github.com/metal-stack/tenant-api/go/api/v1/apiv1connect"
	tenant "github.com/metal-stack/tenant-api/go/client"
	memorydatastore "github.com/metal-stack/tenant-apiserver/pkg/datastore/memory"
	pgdatastore "github.com/metal-stack/tenant-apiserver/pkg/datastore/postgres"
	"github.com/metal-stack/tenant-apiserver/pkg/service"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func StartTenantApiserverWithPostgres(t testing.TB, log *slog.Logger) (tenant.Client, func()) {
	ctx := t.Context()

	postgres, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithPassword("password"),
		postgres.BasicWaitStrategies(),
		testcontainers.WithTmpfs(map[string]string{"/var/lib/postgresql": "rw"}),
		testcontainers.WithName(containerName(t)),
	)
	require.NoError(t, err)

	connectionString, err := postgres.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sqlx.Open("postgres", connectionString)
	require.NoError(t, err)

	closer := func() {
		_ = postgres.Terminate(ctx)
	}

	return startTenantApiserverWithDB(t, log, closer, db)
}

func startTenantApiserverWithDB(t testing.TB, log *slog.Logger, dbcloser func(), db *sqlx.DB) (tenant.Client, func()) {

	log = log.WithGroup("tenant-apiserver")
	ps := pgdatastore.New(log, db, &apiv1.Project{})
	pms := pgdatastore.New(log, db, &apiv1.ProjectMember{})
	ts := pgdatastore.New(log, db, &apiv1.Tenant{})
	tms := pgdatastore.New(log, db, &apiv1.TenantMember{})

	err := pgdatastore.InitTables(log, db,
		&apiv1.Project{},
		&apiv1.ProjectMember{},
		&apiv1.Tenant{},
		&apiv1.TenantMember{},
	)
	require.NoError(t, err)

	projectService := service.NewProjectService(log, ps, pms, ts)
	projectMemberService := service.NewProjectMemberService(log, ps, pms, ts)
	tenantService := service.NewTenantService(nil, log, ts, tms)
	tenantMemberService := service.NewTenantMemberService(log, ts, tms)
	versionService := service.NewVersionService()

	mux := http.NewServeMux()
	mux.Handle(apiv1connect.NewProjectServiceHandler(projectService))
	mux.Handle(apiv1connect.NewProjectMemberServiceHandler(projectMemberService))
	mux.Handle(apiv1connect.NewTenantServiceHandler(tenantService))
	mux.Handle(apiv1connect.NewTenantMemberServiceHandler(tenantMemberService))
	mux.Handle(apiv1connect.NewVersionServiceHandler(versionService))

	server := httptest.NewUnstartedServer(mux)
	server.EnableHTTP2 = true
	server.StartTLS()
	closer := func() {
		server.Close()
	}

	conn, err := tenant.New(&tenant.DialConfig{
		BaseURL:   server.URL,
		Namespace: "metal-stack.io",
	})
	require.NoError(t, err)
	return conn, closer
}

func StartTenantApiserverInMemory(t testing.TB, log *slog.Logger) (tenant.Client, func()) {
	log = log.WithGroup("tenant-apiserver")

	ps := memorydatastore.NewMemory(log, &apiv1.Project{})
	pms := memorydatastore.NewMemory(log, &apiv1.ProjectMember{})
	ts := memorydatastore.NewMemory(log, &apiv1.Tenant{})
	tms := memorydatastore.NewMemory(log, &apiv1.TenantMember{})

	projectService := service.NewProjectService(log, ps, pms, ts)
	projectMemberService := service.NewProjectMemberService(log, ps, pms, ts)
	tenantService := service.NewTenantService(nil, log, ts, tms)
	tenantMemberService := service.NewTenantMemberService(log, ts, tms)
	versionService := service.NewVersionService()

	mux := http.NewServeMux()
	mux.Handle(apiv1connect.NewProjectServiceHandler(projectService))
	mux.Handle(apiv1connect.NewProjectMemberServiceHandler(projectMemberService))
	mux.Handle(apiv1connect.NewTenantServiceHandler(tenantService))
	mux.Handle(apiv1connect.NewTenantMemberServiceHandler(tenantMemberService))
	mux.Handle(apiv1connect.NewVersionServiceHandler(versionService))

	server := httptest.NewUnstartedServer(mux)
	server.EnableHTTP2 = true
	server.StartTLS()
	closer := func() {
		server.Close()
	}

	conn, err := tenant.New(&tenant.DialConfig{
		BaseURL:   server.URL,
		Namespace: "metal-stack.io",
	})
	require.NoError(t, err)

	return conn, closer
}
