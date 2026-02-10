package admin

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
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

func New(c Config) adminv2connect.SizeReservationServiceHandler {
	return &sizeReservationServiceServer{
		log:  c.Log.WithGroup("adminSizeReservationService"),
		repo: c.Repo,
	}
}

func (s *sizeReservationServiceServer) Create(ctx context.Context, req *adminv2.SizeReservationServiceCreateRequest) (*adminv2.SizeReservationServiceCreateResponse, error) {
	sizeReservation, err := s.repo.UnscopedSizeReservation().Create(ctx, req)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.SizeReservationServiceCreateResponse{SizeReservation: sizeReservation}, nil
}

func (s *sizeReservationServiceServer) Delete(ctx context.Context, req *adminv2.SizeReservationServiceDeleteRequest) (*adminv2.SizeReservationServiceDeleteResponse, error) {
	sizeReservation, err := s.repo.UnscopedSizeReservation().Delete(ctx, req.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.SizeReservationServiceDeleteResponse{SizeReservation: sizeReservation}, nil
}

func (s *sizeReservationServiceServer) List(ctx context.Context, req *adminv2.SizeReservationServiceListRequest) (*adminv2.SizeReservationServiceListResponse, error) {
	sizeReservations, err := s.repo.UnscopedSizeReservation().List(ctx, req.Query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.SizeReservationServiceListResponse{SizeReservations: sizeReservations}, nil
}

func (s *sizeReservationServiceServer) Update(ctx context.Context, req *adminv2.SizeReservationServiceUpdateRequest) (*adminv2.SizeReservationServiceUpdateResponse, error) {
	sizeReservation, err := s.repo.UnscopedSizeReservation().Update(ctx, req.Id, req)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.SizeReservationServiceUpdateResponse{SizeReservation: sizeReservation}, nil
}
