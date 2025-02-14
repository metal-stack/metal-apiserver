package filesystem

import (
	"context"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/metal-stack/api-server/pkg/db/metal"
	"github.com/metal-stack/api-server/pkg/repository"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
)

type Config struct {
	Log  *slog.Logger
	Repo *repository.Repository
}

type filesystemServiceServer struct {
	log  *slog.Logger
	repo *repository.Repository
}

func New(c Config) apiv2connect.FilesystemServiceHandler {
	return &filesystemServiceServer{
		log:  c.Log.WithGroup("filesystemService"),
		repo: c.Repo,
	}
}

func (f *filesystemServiceServer) Get(ctx context.Context, rq *connect.Request[apiv2.FilesystemServiceGetRequest]) (*connect.Response[apiv2.FilesystemServiceGetResponse], error) {
	req := rq.Msg
	resp, err := f.repo.FilesystemLayout().Get(ctx, req.Id)
	if err != nil {
		if generic.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, err
	}

	fsl, err := convert(resp)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&apiv2.FilesystemServiceGetResponse{
		FilesystemLayout: fsl,
	}), nil
}

func (f *filesystemServiceServer) List(ctx context.Context, rq *connect.Request[apiv2.FilesystemServiceListRequest]) (*connect.Response[apiv2.FilesystemServiceListResponse], error) {
	panic("unimplemented")
}

func (f *filesystemServiceServer) Match(ctx context.Context, rq *connect.Request[apiv2.FilesystemServiceMatchRequest]) (*connect.Response[apiv2.FilesystemServiceMatchResponse], error) {
	panic("unimplemented")
}

func (f *filesystemServiceServer) Try(ctx context.Context, rq *connect.Request[apiv2.FilesystemServiceTryRequest]) (*connect.Response[apiv2.FilesystemServiceTryResponse], error) {
	panic("unimplemented")
}

// FIXME should be in repo ?
func convert(in *metal.FilesystemLayout) (*apiv2.FilesystemLayout, error) {

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
			Spares:        int32(r.Spares),
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
		Id: in.ID,
		// Name: in.Name,
		// Description: in.Description,
		Filesystems:    filesystems,
		Disks:          disks,
		Raid:           raid,
		VolumeGroups:   volumegroups,
		LogicalVolumes: logicalvolumes,
		Constraints:    constraints,
	}

	return fsl, nil
}
