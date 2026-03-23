package admin

import (
	"context"
	"log/slog"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Store
}

type imageServiceServer struct {
	log  *slog.Logger
	repo *repository.Store
}

func New(c Config) adminv2connect.ImageServiceHandler {
	return &imageServiceServer{
		log:  c.Log.WithGroup("adminImageService"),
		repo: c.Repo,
	}
}

// Create implements adminv2connect.ImageServiceHandler.
func (i *imageServiceServer) Create(ctx context.Context, rq *adminv2.ImageServiceCreateRequest) (*adminv2.ImageServiceCreateResponse, error) {
	image, err := i.repo.Image().Create(ctx, rq)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.ImageServiceCreateResponse{Image: image}, nil
}

// Delete implements adminv2connect.ImageServiceHandler.
func (i *imageServiceServer) Delete(ctx context.Context, rq *adminv2.ImageServiceDeleteRequest) (*adminv2.ImageServiceDeleteResponse, error) {
	image, err := i.repo.Image().Delete(ctx, rq.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.ImageServiceDeleteResponse{Image: image}, nil
}

// Update implements adminv2connect.ImageServiceHandler.
func (i *imageServiceServer) Update(ctx context.Context, rq *adminv2.ImageServiceUpdateRequest) (*adminv2.ImageServiceUpdateResponse, error) {
	image, err := i.repo.Image().Update(ctx, rq.Id, rq)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.ImageServiceUpdateResponse{Image: image}, nil
}

// Usage implements adminv2connect.ImageServiceHandler.
func (i *imageServiceServer) Usage(ctx context.Context, rq *adminv2.ImageServiceUsageRequest) (*adminv2.ImageServiceUsageResponse, error) {
	machineQuery := &apiv2.MachineQuery{}

	if rq.Query != nil {
		machineQuery.Allocation = &apiv2.MachineAllocationQuery{
			Image: rq.Query.Id,
		}
	}

	machines, err := i.repo.UnscopedMachine().List(ctx, machineQuery)
	if err != nil {
		return nil, err
	}

	machinesByImage := map[string][]string{}
	for _, m := range machines {
		if m.Allocation == nil || m.Allocation.Image == nil {
			continue
		}

		machinesByImage[m.Allocation.Image.Id] = append(machinesByImage[m.Allocation.Image.Id], m.Uuid)
	}

	images, err := i.repo.Image().List(ctx, rq.Query)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	var usage []*apiv2.ImageUsage

	for _, img := range images {
		usage = append(usage, &apiv2.ImageUsage{
			Image:  img,
			UsedBy: machinesByImage[img.Id],
		})
	}

	return &adminv2.ImageServiceUsageResponse{
		ImageUsage: usage,
	}, nil
}
