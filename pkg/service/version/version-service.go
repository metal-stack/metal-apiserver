package version

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/v"
)

type Config struct {
	Log *slog.Logger
}
type versionServiceServer struct {
	log *slog.Logger
}

func New(c Config) apiv2connect.VersionServiceHandler {
	return &versionServiceServer{
		log: c.Log.WithGroup("versionService"),
	}
}

func (a *versionServiceServer) Get(ctx context.Context, rq *connect.Request[apiv2.VersionServiceGetRequest]) (*connect.Response[apiv2.VersionServiceGetResponse], error) {
	version := &apiv2.Version{
		Version:   v.Version,
		Revision:  v.Revision,
		GitSha1:   v.GitSHA1,
		BuildDate: v.BuildDate,
	}
	return connect.NewResponse(&apiv2.VersionServiceGetResponse{
		Version: version,
	}), nil
}
