package infra

import (
	"context"
	"errors"
	"log/slog"

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

func (s *eventServiceServer) Send(ctx context.Context, rq *infrav2.EventServiceSendRequest) (*infrav2.EventServiceSendResponse, error) {
	s.log.Debug("send", "event", rq)

	var (
		processed uint64
		failed    []string
		errs      []error
	)

	for id, event := range rq.Events {
		err := s.repo.UnscopedMachine().AdditionalMethods().SendEvent(ctx, s.log, id, event)
		if err != nil {
			errs = append(errs, err)
			failed = append(failed, id)
			continue
		}
		processed++
	}

	return &infrav2.EventServiceSendResponse{
		Events: processed,
		Failed: failed,
	}, errors.Join(errs...)
}
