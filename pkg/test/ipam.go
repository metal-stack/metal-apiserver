package test

import (
	"log/slog"
	"os"
	"testing"

	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	ipamtest "github.com/metal-stack/go-ipam/pkg/test"
)

func StartIpam(t testing.TB) (ipamv1connect.IpamServiceClient, func()) {
	var (
		ctx = t.Context()
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	)

	ipamclient, closer := ipamtest.NewTestServer(ctx, log)

	return ipamclient, closer
}
