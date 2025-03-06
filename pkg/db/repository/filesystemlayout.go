package repository

import (
	"context"
	"fmt"

	"github.com/metal-stack/api-server/pkg/db/metal"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

type filesystemLayoutRepository struct {
	r *Store
}

func (r *filesystemLayoutRepository) ValidateCreate(ctx context.Context, req *adminv2.FilesystemServiceCreateRequest) (*Validated[*adminv2.FilesystemServiceCreateRequest], error) {
	return &Validated[*adminv2.FilesystemServiceCreateRequest]{
		message: req,
	}, nil
}

func (r *filesystemLayoutRepository) ValidateUpdate(ctx context.Context, req *adminv2.FilesystemServiceUpdateRequest) (*Validated[*adminv2.FilesystemServiceUpdateRequest], error) {
	return &Validated[*adminv2.FilesystemServiceUpdateRequest]{
		message: req,
	}, nil
}
func (r *filesystemLayoutRepository) ValidateDelete(ctx context.Context, req *metal.FilesystemLayout) (*Validated[*metal.FilesystemLayout], error) {
	return &Validated[*metal.FilesystemLayout]{
		message: req,
	}, nil
}

func (r *filesystemLayoutRepository) Get(ctx context.Context, id string) (*metal.FilesystemLayout, error) {
	fsl, err := r.r.ds.FilesystemLayout().Get(ctx, id)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	resp, err := r.r.ds.FilesystemLayout().Create(ctx, fsl)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (r *filesystemLayoutRepository) Update(ctx context.Context, rq *Validated[*adminv2.FilesystemServiceUpdateRequest]) (*metal.FilesystemLayout, error) {
	old, err := r.Get(ctx, rq.message.FilesystemLayout.Id)
	if err != nil {
		return nil, err
	}

	new := *old

	// FIXME implement update logic

	err = r.r.ds.FilesystemLayout().Update(ctx, &new, old)
	if err != nil {
		return nil, err
	}

	return &new, nil
}

func (r *filesystemLayoutRepository) Delete(ctx context.Context, rq *Validated[*metal.FilesystemLayout]) (*metal.FilesystemLayout, error) {
	fsl, err := r.Get(ctx, rq.message.ID)
	if err != nil {
		return nil, err
	}

	err = r.r.ds.FilesystemLayout().Delete(ctx, fsl)
	if err != nil {
		return nil, err
	}

	return fsl, nil
}

func (r *filesystemLayoutRepository) Find(ctx context.Context, rq *apiv2.FilesystemServiceListRequest) (*metal.FilesystemLayout, error) {
	panic("unimplemented")
}

func (r *filesystemLayoutRepository) List(ctx context.Context, rq *apiv2.FilesystemServiceListRequest) ([]*metal.FilesystemLayout, error) {
	ip, err := r.r.ds.FilesystemLayout().List(ctx)
	if err != nil {
		return nil, err
	}

	return ip, nil
}

func (r *filesystemLayoutRepository) ConvertToInternal(msg *apiv2.FilesystemLayout) (*metal.FilesystemLayout, error) {
	panic("unimplemented")
}
func (r *filesystemLayoutRepository) ConvertToProto(in *metal.FilesystemLayout) (*apiv2.FilesystemLayout, error) {

	var filesystems []*apiv2.Filesystem
	for _, fs := range in.Filesystems {
		var f apiv2.Format
		switch fs.Format {
		case metal.NONE:
			f = apiv2.Format_FORMAT_NONE
		case metal.EXT3:
			f = apiv2.Format_FORMAT_EXT3
		case metal.EXT4:
			f = apiv2.Format_FORMAT_EXT4
		case metal.SWAP:
			f = apiv2.Format_FORMAT_SWAP
		case metal.TMPFS:
			f = apiv2.Format_FORMAT_TMPFS
		case metal.VFAT:
			f = apiv2.Format_FORMAT_VFAT
		default:
			return nil, fmt.Errorf("unknown filesystem format:%s", fs.Format)
		}

		filesystems = append(filesystems, &apiv2.Filesystem{
			Device: fs.Device,
			Format: f,
		})
	}
	var disks []*apiv2.Disk
	for _, d := range in.Disks {
		var partitions []*apiv2.DiskPartition
		for _, p := range d.Partitions {
			var gpt *apiv2.GPTType
			if p.GPTType != nil {
				switch *p.GPTType {
				case metal.GPTBoot:
					gpt = apiv2.GPTType_GPT_TYPE_BOOT.Enum()
				case metal.GPTLinux:
					gpt = apiv2.GPTType_GPT_TYPE_LINUX.Enum()
				case metal.GPTLinuxLVM:
					gpt = apiv2.GPTType_GPT_TYPE_LINUX_LVM.Enum()
				case metal.GPTLinuxRaid:
					gpt = apiv2.GPTType_GPT_TYPE_LINUX_RAID.Enum()
				default:
					return nil, fmt.Errorf("unknown gpttype:%s", *p.GPTType)
				}
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
		var lvmType apiv2.LVMType
		switch lv.LVMType {
		case metal.LVMTypeLinear:
			lvmType = apiv2.LVMType_LVM_TYPE_LINEAR
		case metal.LVMTypeRaid1:
			lvmType = apiv2.LVMType_LVM_TYPE_RAID1
		case metal.LVMTypeStriped:
			lvmType = apiv2.LVMType_LVM_TYPE_STRIPED
		default:
			return nil, fmt.Errorf("unknown lvm type:%s", lv.LVMType)
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
