package repository

import (
	"context"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

func (r *filesystemLayoutRepository) validateCreate(ctx context.Context, req *adminv2.FilesystemServiceCreateRequest) error {
	fsl, err := r.convertToInternal(ctx, req.FilesystemLayout)
	if err != nil {
		return errorutil.Convert(err)
	}

	err = fsl.Validate()
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}

func (r *filesystemLayoutRepository) validateUpdate(ctx context.Context, req *adminv2.FilesystemServiceUpdateRequest, _ *metal.FilesystemLayout) error {
	filesystemLayout := &apiv2.FilesystemLayout{
		Id:             req.Id,
		Name:           req.Name,
		Description:    req.Description,
		Filesystems:    req.Filesystems,
		Disks:          req.Disks,
		Raid:           req.Raid,
		VolumeGroups:   req.VolumeGroups,
		LogicalVolumes: req.LogicalVolumes,
		Constraints:    req.Constraints,
	}

	fsl, err := r.convertToInternal(ctx, filesystemLayout)
	if err != nil {
		return errorutil.Convert(err)
	}

	var allFsls metal.FilesystemLayouts
	fsls, err := r.list(ctx, &apiv2.FilesystemServiceListRequest{})
	if err != nil {
		return errorutil.Convert(err)
	}
	allFsls = append(allFsls, fsls...)

	allFsls = append(allFsls, fsl)
	err = allFsls.Validate()
	if err != nil {
		return errorutil.Convert(err)
	}

	err = fsl.Validate()
	if err != nil {
		return errorutil.Convert(err)
	}

	return nil
}

func (r *filesystemLayoutRepository) validateDelete(ctx context.Context, fsl *metal.FilesystemLayout) error {
	machines, err := r.s.UnscopedMachine().List(ctx, &apiv2.MachineQuery{
		Allocation: &apiv2.MachineAllocationQuery{FilesystemLayout: &fsl.ID},
	})
	if err != nil {
		return err
	}
	if len(machines) > 0 {
		return errorutil.InvalidArgument("cannot remove filesystemlayout with existing machine allocations")
	}
	return nil
}
