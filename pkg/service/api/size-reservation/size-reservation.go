package sizereservation

import (
	"context"
	"log/slog"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type sizeReservationServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) apiv2connect.SizeReservationServiceHandler {
	return &sizeReservationServiceServer{
		log:  c.Log.WithGroup("sizeReservationService"),
		repo: c.Repo,
	}
}

func (s *sizeReservationServiceServer) Get(ctx context.Context, req *apiv2.SizeReservationServiceGetRequest) (*apiv2.SizeReservationServiceGetResponse, error) {
	sizeReservation, err := s.repo.SizeReservation(req.Project).Get(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &apiv2.SizeReservationServiceGetResponse{SizeReservation: sizeReservation}, nil
}

func (s *sizeReservationServiceServer) List(ctx context.Context, req *apiv2.SizeReservationServiceListRequest) (*apiv2.SizeReservationServiceListResponse, error) {
	sizeReservations, err := s.repo.SizeReservation(req.Project).List(ctx, req.Query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &apiv2.SizeReservationServiceListResponse{SizeReservations: sizeReservations}, nil
}
