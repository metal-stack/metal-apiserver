package version

import (
	"context"
	"log/slog"

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

func (a *versionServiceServer) Get(ctx context.Context, rq *apiv2.VersionServiceGetRequest) (*apiv2.VersionServiceGetResponse, error) {
	version := &apiv2.Version{
		Version:   v.Version,
		Revision:  v.Revision,
		GitSha1:   v.GitSHA1,
		BuildDate: v.BuildDate,
	}
	return &apiv2.VersionServiceGetResponse{
		Version: version,
	}, nil
}
