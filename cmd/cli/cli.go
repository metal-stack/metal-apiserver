package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"connectrpc.com/connect"
	"github.com/metal-stack/api/go/client"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

func main() {

	dialConfig := client.DialConfig{
		BaseURL:   "http://localhost:8081",
		Token:     os.Args[1],
		UserAgent: "metal-stack-cli",
		Debug:     true,
	}

	ac := client.New(dialConfig)
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := context.Background()

	version, err := ac.Apiv2().Version().Get(ctx, connect.NewRequest(&apiv2.VersionServiceGetRequest{}))
	if err != nil {
		panic(err)
	}
	log.Info("cli", "version", version.Msg)

	tenantResp, err := ac.Adminv2().Tenant().Create(ctx, connect.NewRequest(&adminv2.TenantServiceCreateRequest{Name: "metal-apiserver-cli"}))
	if err != nil {
		var connectErr *connect.Error
		if !errors.As(err, &connectErr) {
			panic(err)
		}
		if connectErr.Code() != connect.CodeAlreadyExists {
			panic(err)
		}
	}
	log.Info("tenant created", "tenant", tenantResp.Msg)

	projectResp, err := ac.Apiv2().Project().Create(ctx, connect.NewRequest(&apiv2.ProjectServiceCreateRequest{Login: "metal-apiserver-cli", Name: "my project", Description: "Sample Project"}))
	if err != nil {
		var connectErr *connect.Error
		if !errors.As(err, &connectErr) {
			panic(err)
		}
		if connectErr.Code() != connect.CodeAlreadyExists {
			panic(err)
		}
	}
	log.Info("project", "created", projectResp.Msg.Project)

	projectListResp, err := ac.Apiv2().Project().List(ctx, connect.NewRequest(&apiv2.ProjectServiceListRequest{Tenant: pointer.Pointer("metal-apiserver-cli")}))
	if err != nil {
		panic(err)
	}
	log.Info("projects", "projects", projectListResp.Msg.Projects)

	pid := projectListResp.Msg.Projects[0].Uuid

	npr, err := ac.Apiv2().Network().Create(ctx, connect.NewRequest(&apiv2.NetworkServiceCreateRequest{Id: pointer.Pointer("internet"), Prefixes: []string{"10.0.0.0/16"}}))
	if err != nil {
		var connectErr *connect.Error
		if !errors.As(err, &connectErr) {
			panic(err)
		}
		if connectErr.Code() != connect.CodeAlreadyExists {
			panic(err)
		}
	}
	log.Info("network created", "nw", npr.Msg)

	ipr, err := ac.Apiv2().IP().Create(ctx, connect.NewRequest(&apiv2.IPServiceCreateRequest{Project: pid, Network: "internet"}))
	if err != nil {
		panic(err)
	}

	log.Info("ip created", "ip", ipr.Msg)

}
