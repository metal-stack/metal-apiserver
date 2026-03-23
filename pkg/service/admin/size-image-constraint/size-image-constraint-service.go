package admin

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type sizeImageConstraintServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.SizeImageConstraintServiceHandler {
	return &sizeImageConstraintServiceServer{
		log:  c.Log.WithGroup("adminSizeImageConstraintService"),
		repo: c.Repo,
	}
}

func (s *sizeImageConstraintServiceServer) Create(ctx context.Context, req *adminv2.SizeImageConstraintServiceCreateRequest) (*adminv2.SizeImageConstraintServiceCreateResponse, error) {
	sic, err := s.repo.SizeImageConstraint().Create(ctx, req)
	if err != nil {
		return nil, err
	}
	return &adminv2.SizeImageConstraintServiceCreateResponse{
		SizeImageConstraint: sic,
	}, nil
}

func (s *sizeImageConstraintServiceServer) Delete(ctx context.Context, req *adminv2.SizeImageConstraintServiceDeleteRequest) (*adminv2.SizeImageConstraintServiceDeleteResponse, error) {
	sic, err := s.repo.SizeImageConstraint().Delete(ctx, req.Size)
	if err != nil {
		return nil, err
	}
	return &adminv2.SizeImageConstraintServiceDeleteResponse{
		SizeImageConstraint: sic,
	}, nil
}

func (s *sizeImageConstraintServiceServer) Get(ctx context.Context, req *adminv2.SizeImageConstraintServiceGetRequest) (*adminv2.SizeImageConstraintServiceGetResponse, error) {
	sic, err := s.repo.SizeImageConstraint().Get(ctx, req.Size)
	if err != nil {
		return nil, err
	}
	return &adminv2.SizeImageConstraintServiceGetResponse{
		SizeImageConstraint: sic,
	}, nil
}

func (s *sizeImageConstraintServiceServer) List(ctx context.Context, req *adminv2.SizeImageConstraintServiceListRequest) (*adminv2.SizeImageConstraintServiceListResponse, error) {
	sics, err := s.repo.SizeImageConstraint().List(ctx, req.Query)
	if err != nil {
		return nil, err
	}
	return &adminv2.SizeImageConstraintServiceListResponse{
		SizeImageConstraints: sics,
	}, nil
}

func (s *sizeImageConstraintServiceServer) Update(ctx context.Context, req *adminv2.SizeImageConstraintServiceUpdateRequest) (*adminv2.SizeImageConstraintServiceUpdateResponse, error) {
	sic, err := s.repo.SizeImageConstraint().Update(ctx, req.Size, req)
	if err != nil {
		return nil, err
	}
	return &adminv2.SizeImageConstraintServiceUpdateResponse{
		SizeImageConstraint: sic,
	}, nil
}
