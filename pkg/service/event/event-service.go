package event

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
	"github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type eventServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) infrav2connect.EventServiceHandler {
	return &eventServiceServer{
		log:  c.Log.WithGroup("eventService"),
		repo: c.Repo,
	}
}

// Send implements infrav2connect.EventServiceHandler.
func (e *eventServiceServer) Send(ctx context.Context, rq *connect.Request[infrav2.EventServiceSendRequest]) (*connect.Response[infrav2.EventServiceSendResponse], error) {
	panic("unimplemented")
}

// SendMulti implements infrav2connect.EventServiceHandler.
func (e *eventServiceServer) SendMulti(ctx context.Context,rq *connect.Request[infrav2.EventServiceSendMultiRequest]) (*connect.Response[infrav2.EventServiceSendMultiResponse], error) {
	panic("unimplemented")
}
