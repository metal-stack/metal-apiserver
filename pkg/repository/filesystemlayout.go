package repository

import (
	"context"
	"fmt"

	"github.com/metal-stack/api/go/enum"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

type filesystemLayoutRepository struct {
	r *Store
}

func (r *filesystemLayoutRepository) ValidateCreate(ctx context.Context, req *adminv2.FilesystemServiceCreateRequest) (*Validated[*adminv2.FilesystemServiceCreateRequest], error) {
	fsl, err := r.ConvertToInternal(req.FilesystemLayout)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	err = fsl.Validate()
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return &Validated[*adminv2.FilesystemServiceCreateRequest]{
		message: req,
	}, nil
}

func (r *filesystemLayoutRepository) ValidateUpdate(ctx context.Context, req *adminv2.FilesystemServiceUpdateRequest) (*Validated[*adminv2.FilesystemServiceUpdateRequest], error) {
	fsl, err := r.ConvertToInternal(req.FilesystemLayout)
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	err = fsl.Validate()
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	return &Validated[*adminv2.FilesystemServiceUpdateRequest]{
		message: req,
	}, nil
}
func (r *filesystemLayoutRepository) ValidateDelete(ctx context.Context, req *metal.FilesystemLayout) (*Validated[*metal.FilesystemLayout], error) {
	// FIXME implement a lookup if any machine uses this fsl
	return &Validated[*metal.FilesystemLayout]{
		message: req,
	}, nil
}

func (r *filesystemLayoutRepository) Get(ctx context.Context, id string) (*metal.FilesystemLayout, error) {
	fsl, err := r.r.ds.FilesystemLayout().Get(ctx, id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return fsl, nil
}

// Filesystem is not project scoped
func (r *filesystemLayoutRepository) MatchScope(_ *metal.FilesystemLayout) error {
	return nil
}

func (r *filesystemLayoutRepository) Create(ctx context.Context, rq *Validated[*adminv2.FilesystemServiceCreateRequest]) (*metal.FilesystemLayout, error) {
	fsl, err := r.ConvertToInternal(rq.message.FilesystemLayout)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	resp, err := r.r.ds.FilesystemLayout().Create(ctx, fsl)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp, nil
}

func (r *filesystemLayoutRepository) Update(ctx context.Context, rq *Validated[*adminv2.FilesystemServiceUpdateRequest]) (*metal.FilesystemLayout, error) {
	old, err := r.Get(ctx, rq.message.FilesystemLayout.Id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	var allFsls metal.FilesystemLayouts
	fsls, err := r.List(ctx, &apiv2.FilesystemServiceListRequest{})
	if err != nil {
		return nil, errorutil.Convert(err)
	}
	allFsls = append(allFsls, fsls...)

	newFsl, err := r.ConvertToInternal(rq.message.FilesystemLayout)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	allFsls = append(allFsls, newFsl)
	err = allFsls.Validate()
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	newFsl.SetChanged(old.Changed)

	// FIXME implement update logic

	err = r.r.ds.FilesystemLayout().Update(ctx, newFsl)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return newFsl, nil
}

func (r *filesystemLayoutRepository) Delete(ctx context.Context, rq *Validated[*metal.FilesystemLayout]) (*metal.FilesystemLayout, error) {
	fsl, err := r.Get(ctx, rq.message.ID)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	err = r.r.ds.FilesystemLayout().Delete(ctx, fsl)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return fsl, nil
}

func (r *filesystemLayoutRepository) Find(ctx context.Context, rq *apiv2.FilesystemServiceListRequest) (*metal.FilesystemLayout, error) {
	panic("unimplemented")
}

func (r *filesystemLayoutRepository) List(ctx context.Context, rq *apiv2.FilesystemServiceListRequest) ([]*metal.FilesystemLayout, error) {
	fsls, err := r.r.ds.FilesystemLayout().List(ctx)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return fsls, nil
}

func (r *filesystemLayoutRepository) ConvertToInternal(f *apiv2.FilesystemLayout) (*metal.FilesystemLayout, error) {
	var (
		fss = []metal.Filesystem{}
		ds  = []metal.Disk{}
		rs  = []metal.Raid{}
		vgs = []metal.VolumeGroup{}
		lvs = []metal.LogicalVolume{}
	)
	for _, fs := range f.Filesystems {
		formatString, err := enum.GetStringValue(fs.Format)
		if err != nil {
			return nil, err
		}
		format, err := metal.ToFormat(*formatString)
		if err != nil {
			return nil, err
		}
		v1fs := metal.Filesystem{
			Path:          fs.Path,
			Device:        string(fs.Device),
			Format:        *format,
			Label:         fs.Label,
			MountOptions:  fs.MountOptions,
			CreateOptions: fs.CreateOptions,
		}
		fss = append(fss, v1fs)
	}
	for _, disk := range f.Disks {
		parts := []metal.DiskPartition{}
		for _, p := range disk.Partitions {
			part := metal.DiskPartition{
				Number: uint8(p.Number), // nolint:gosec
				Size:   p.Size,
				Label:  p.Label,
			}
			if p.GptType != nil {
				gptTypeString, err := enum.GetStringValue(p.GptType)
				if err != nil {
					return nil, err
				}
				gptType, err := metal.ToGPTType(*gptTypeString)
				if err != nil {
					return nil, err
				}
				part.GPTType = gptType
			}
			parts = append(parts, part)
		}
		d := metal.Disk{
			Device:          string(disk.Device),
			Partitions:      parts,
			WipeOnReinstall: disk.WipeOnReinstall,
		}
		ds = append(ds, d)
	}
	for _, raid := range f.Raid {
		raidLevelString, err := enum.GetStringValue(raid.Level)
		if err != nil {
			return nil, err
		}
		level, err := metal.ToRaidLevel(*raidLevelString)
		if err != nil {
			return nil, err
		}
		r := metal.Raid{
			ArrayName:     raid.ArrayName,
			Devices:       raid.Devices,
			Level:         *level,
			CreateOptions: raid.CreateOptions,
			Spares:        int(raid.Spares),
		}
		rs = append(rs, r)
	}
	for _, v := range f.VolumeGroups {
		vg := metal.VolumeGroup{
			Name:    v.Name,
			Devices: v.Devices,
			Tags:    v.Tags,
		}
		vgs = append(vgs, vg)
	}
	for _, l := range f.LogicalVolumes {
		lvmtypeString, err := enum.GetStringValue(l.LvmType)
		if err != nil {
			return nil, err
		}
		lvmtype, err := metal.ToLVMType(*lvmtypeString)
		if err != nil {
			return nil, err
		}
		lv := metal.LogicalVolume{
			Name:        l.Name,
			VolumeGroup: l.VolumeGroup,
			Size:        l.Size,
			LVMType:     *lvmtype,
		}
		lvs = append(lvs, lv)
	}

	constraint := metal.FilesystemLayoutConstraints{}
	if f.Constraints != nil {
		constraint.Images = f.Constraints.Images
		constraint.Sizes = f.Constraints.Sizes
	}
	fl := &metal.FilesystemLayout{
		Base: metal.Base{
			ID: f.Id,
		},
		Filesystems:    fss,
		Disks:          ds,
		Raid:           rs,
		VolumeGroups:   vgs,
		LogicalVolumes: lvs,
		Constraints:    constraint,
	}
	if f.Name != nil {
		fl.Name = *f.Name
	}
	if f.Description != nil {
		fl.Description = *f.Description
	}
	return fl, nil

}
func (r *filesystemLayoutRepository) ConvertToProto(in *metal.FilesystemLayout) (*apiv2.FilesystemLayout, error) {

	var filesystems []*apiv2.Filesystem
	for _, fs := range in.Filesystems {
		f, err := enum.GetEnum[apiv2.Format](string(fs.Format))
		if err != nil {
			return nil, err
		}
		filesystems = append(filesystems, &apiv2.Filesystem{
			Device: fs.Device,
			Format: f,
			Label:  fs.Label,
			Path:   fs.Path,
		})
	}
	var disks []*apiv2.Disk
	for _, d := range in.Disks {
		var partitions []*apiv2.DiskPartition
		for _, p := range d.Partitions {
			var gpt *apiv2.GPTType
			if p.GPTType != nil {
				gptParsed, err := enum.GetEnum[apiv2.GPTType](string(*p.GPTType))
				if err != nil {
					return nil, err
				}
				gpt = &gptParsed
			}

			partitions = append(partitions, &apiv2.DiskPartition{
				Number:  uint32(p.Number),
				Label:   p.Label,
				Size:    p.Size,
				GptType: gpt,
			})
		}
		disks = append(disks, &apiv2.Disk{
			Device:          d.Device,
			Partitions:      partitions,
			WipeOnReinstall: d.WipeOnReinstall,
		})
	}

	var raid []*apiv2.Raid
	for _, r := range in.Raid {
		var level apiv2.RaidLevel
		switch r.Level {
		case metal.RaidLevel0:
			level = apiv2.RaidLevel_RAID_LEVEL_0
		case metal.RaidLevel1:
			level = apiv2.RaidLevel_RAID_LEVEL_1
		default:
			return nil, fmt.Errorf("unknown raid level:%s", r.Level)
		}
		raid = append(raid, &apiv2.Raid{
			ArrayName:     r.ArrayName,
			Devices:       r.Devices,
			Level:         level,
			CreateOptions: r.CreateOptions,
			Spares:        int32(r.Spares), // nolint:gosec
		})
	}

	var volumegroups []*apiv2.VolumeGroup
	for _, vg := range in.VolumeGroups {
		volumegroups = append(volumegroups, &apiv2.VolumeGroup{
			Name:    vg.Name,
			Devices: vg.Devices,
			Tags:    vg.Tags,
		})
	}

	var logicalvolumes []*apiv2.LogicalVolume
	for _, lv := range in.LogicalVolumes {
		lvmType, err := enum.GetEnum[apiv2.LVMType](string(lv.LVMType))
		if err != nil {
			return nil, err
		}
		logicalvolumes = append(logicalvolumes, &apiv2.LogicalVolume{
			Name:        lv.Name,
			VolumeGroup: lv.VolumeGroup,
			Size:        lv.Size,
			LvmType:     lvmType,
		})
	}

	constraints := &apiv2.FilesystemLayoutConstraints{
		Sizes:  in.Constraints.Sizes,
		Images: in.Constraints.Images,
	}

	fsl := &apiv2.FilesystemLayout{
		Id:             in.ID,
		Name:           &in.Name,
		Description:    &in.Description,
		Filesystems:    filesystems,
		Disks:          disks,
		Raid:           raid,
		VolumeGroups:   volumegroups,
		LogicalVolumes: logicalvolumes,
		Constraints:    constraints,
	}

	return fsl, nil
}
