package headscale

import (
	"context"
	"fmt"
	"log/slog"

	headscalev1 "github.com/juanfont/headscale/gen/go/headscale/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Config struct {
	Log      *slog.Logger
	Disabled bool
	Apikey   string
	Endpoint string
}

func NewClient(cfg Config) (headscalev1.HeadscaleServiceClient, error) {
	if cfg.Disabled {
		cfg.Log.Info("headscale is not enabled, not configuring vpn services")
		return nil, nil
	}

	grpcOptions := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(tokenAuth{
			token: cfg.Apikey,
		}),
	}

	conn, err := grpc.NewClient(cfg.Endpoint, grpcOptions...)
	if err != nil {
		return nil, fmt.Errorf("unable to create grpc client:%w", err)
	}
	client := headscalev1.NewHeadscaleServiceClient(conn)

	return client, nil
}

type tokenAuth struct {
	token string
}

func (t tokenAuth) GetRequestMetadata(
	ctx context.Context,
	_ ...string,
) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + t.token,
	}, nil
}

func (tokenAuth) RequireTransportSecurity() bool {
	return false
}
