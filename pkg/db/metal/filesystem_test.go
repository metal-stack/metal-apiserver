package metal

import (
	"reflect"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

var (
	s1 = "c1-large-x86"
	s2 = "c1-xlarge-x86"
	s3 = "s3-large-x86"
	i1 = "debian-10"
	i2 = "debian-10.0.20210101"
	i3 = "firewall-2"
	i4 = "centos-7"

	GPTInvalid = GPTType("ff00")
)

func TestFilesystemLayoutConstraint_Matches(t *testing.T) {
	type constraints struct {
		Sizes  []string
		Images map[string]string
	}
	type args struct {
		size  string
		image string
	}
	tests := []struct {
		name string
		c    constraints
		args args
		want bool
	}{
		{
			name: "default layout",
			c: constraints{
				Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
				Images: map[string]string{"ubuntu": "*", "debian": "*"},
			},
			args: args{
				size:  s1,
				image: i1,
			},
			want: true,
		},
		{
			name: "default layout specific image",
			c: constraints{
				Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
				Images: map[string]string{"ubuntu": "*", "debian": "*"},
			},
			args: args{
				size:  s1,
				image: i2,
			},
			want: true,
		},
		{
			name: "default layout specific image constraint",
			c: constraints{
				Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
				Images: map[string]string{"ubuntu": "*", "debian": ">= 10.0.20210101"},
			},
			args: args{
				size:  s1,
				image: i2,
			},
			want: true,
		},
		{
			name: "default layout specific image constraint no match",
			c: constraints{
				Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
				Images: map[string]string{"ubuntu": "*", "debian": ">= 10.0.20210201"},
			},
			args: args{
				size:  s1,
				image: i2,
			},
			want: false,
		},
		{
			name: "firewall layout no match",
			c: constraints{
				Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
				Images: map[string]string{"firewall": "*"},
			},
			args: args{
				size:  s2,
				image: i1,
			},
			want: false,
		},
		{
			name: "firewall layout match",
			c: constraints{
				Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
				Images: map[string]string{"firewall": "*"},
			},
			args: args{
				size:  s2,
				image: i3,
			},
			want: true,
		},
		{
			name: "firewall more specific layout match",
			c: constraints{
				Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
				Images: map[string]string{"firewall": ">= 2"},
			},
			args: args{
				size:  s2,
				image: i3,
			},
			want: true,
		},
		{
			name: "firewall more specific layout no match",
			c: constraints{
				Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
				Images: map[string]string{"firewall": ">= 3"},
			},
			args: args{
				size:  s2,
				image: i3,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			c := &FilesystemLayoutConstraints{
				Sizes:  tt.c.Sizes,
				Images: tt.c.Images,
			}
			if got := c.matches(tt.args.size, tt.args.image); got != tt.want {
				t.Errorf("FilesystemLayoutConstraint.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilesystemLayouts_From(t *testing.T) {
	type args struct {
		size  string
		image string
	}
	tests := []struct {
		name    string
		fls     FilesystemLayouts
		args    args
		want    *string
		wantErr bool
	}{
		{
			name: "simple match debian",
			fls: FilesystemLayouts{
				{
					Base: Base{ID: "default"},
					Constraints: FilesystemLayoutConstraints{
						Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
						Images: map[string]string{"ubuntu": "*", "debian": "*"},
					},
				},
				{
					Base: Base{ID: "firewall"},
					Constraints: FilesystemLayoutConstraints{
						Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
						Images: map[string]string{"firewall": "*"},
					},
				},
			},
			args: args{
				size:  s1,
				image: i1,
			},
			want:    strPtr("default"),
			wantErr: false,
		},
		{
			name: "simple match firewall",
			fls: FilesystemLayouts{
				{
					Base: Base{ID: "default"},
					Constraints: FilesystemLayoutConstraints{
						Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
						Images: map[string]string{"ubuntu": "*", "debian": "*"},
					},
				},
				{
					Base: Base{ID: "firewall"},
					Constraints: FilesystemLayoutConstraints{
						Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
						Images: map[string]string{"firewall": "*"},
					},
				},
			},
			args: args{
				size:  s1,
				image: i3,
			},
			want:    strPtr("firewall"),
			wantErr: false,
		},
		{
			name: "no match, wrong size",
			fls: FilesystemLayouts{
				{
					Base: Base{ID: "default"},
					Constraints: FilesystemLayoutConstraints{
						Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
						Images: map[string]string{"ubuntu": "*", "debian": "*"},
					},
				},
				{
					Base: Base{ID: "firewall"},
					Constraints: FilesystemLayoutConstraints{
						Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
						Images: map[string]string{"firewall": "*"},
					},
				},
			},
			args: args{
				size:  s3,
				image: i1,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "no match, wrong image",
			fls: FilesystemLayouts{
				{
					Base: Base{ID: "default"},
					Constraints: FilesystemLayoutConstraints{
						Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
						Images: map[string]string{"ubuntu": "*", "debian": "*"},
					},
				},
				{
					Base: Base{ID: "firewall"},
					Constraints: FilesystemLayoutConstraints{
						Sizes:  []string{"c1-large-x86", "c1-xlarge-x86"},
						Images: map[string]string{"firewall": "*"},
					},
				},
			},
			args: args{
				size:  s1,
				image: i4,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fls.From(tt.args.size, tt.args.image)
			if (err != nil) != tt.wantErr {
				t.Errorf("FilesystemLayouts.From() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got == nil {
				if tt.want != nil {
					t.Errorf("FilesystemLayouts.From() got nil was not expected")
				}
				return
			}
			if !reflect.DeepEqual(got.ID, *tt.want) {
				t.Errorf("FilesystemLayouts.From() = %v, want %v", got, tt.want)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}

// func TestFilesystemLayout_Matches(t *testing.T) {
// 	type fields struct {
// 		Disks []Disk
// 		Raid  []Raid
// 	}
// 	type args struct {
// 		hardware MachineHardware
// 	}
// 	tests := []struct {
// 		name      string
// 		fields    fields
// 		args      args
// 		wantErr   bool
// 		errString string
// 	}{
// 		{
// 			name: "simple match",
// 			fields: fields{
// 				Disks: []Disk{{Device: "/dev/sda"}, {Device: "/dev/sdb"}},
// 			},
// 			args:    args{hardware: MachineHardware{Disks: []BlockDevice{{Name: "/dev/sda"}, {Name: "/dev/sdb"}}}},
// 			wantErr: false,
// 		},
// 		{
// 			name: "simple match with old device naming",
// 			fields: fields{
// 				Disks: []Disk{{Device: "/dev/sda"}, {Device: "/dev/sdb"}},
// 			},
// 			args:    args{hardware: MachineHardware{Disks: []BlockDevice{{Name: "sda"}, {Name: "sdb"}}}},
// 			wantErr: false,
// 		},
// 		{
// 			name: "simple no match device missing",
// 			fields: fields{
// 				Disks: []Disk{{Device: "/dev/sda"}, {Device: "/dev/sdb"}},
// 			},
// 			args:      args{hardware: MachineHardware{Disks: []BlockDevice{{Name: "/dev/sda"}, {Name: "/dev/sdc"}}}},
// 			wantErr:   true,
// 			errString: "device:/dev/sdb does not exist on given hardware",
// 		},
// 		{
// 			name: "simple no match device to small",
// 			fields: fields{
// 				Disks: []Disk{
// 					{Device: "/dev/sda", Partitions: []DiskPartition{{Size: 100}, {Size: 100}}},
// 					{Device: "/dev/sdb", Partitions: []DiskPartition{{Size: 100}, {Size: 100}}}},
// 			},
// 			args: args{hardware: MachineHardware{Disks: []BlockDevice{
// 				{Name: "/dev/sda", Size: 300000000},
// 				{Name: "/dev/sdb", Size: 100000000},
// 			}}},
// 			wantErr:   true,
// 			errString: "device:/dev/sdb is not big enough required:200MiB, existing:95MiB",
// 		},
// 	}
// 	for _, tt := range tests {
// 		tt := tt
// 		t.Run(tt.name, func(t *testing.T) {
// 			fl := &FilesystemLayout{
// 				Disks: tt.fields.Disks,
// 				Raid:  tt.fields.Raid,
// 			}
// 			err := fl.Matches(tt.args.hardware)
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("FilesystemLayout.Matches() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
// 			if (err != nil) && err.Error() != tt.errString {
// 				t.Errorf("FilesystemLayout.Matches() error = %v, errString %v", err, tt.errString)
// 				return
// 			}
// 		})
// 	}
// }

func TestFilesystemLayout_Validate(t *testing.T) {
	type fields struct {
		Constraints    FilesystemLayoutConstraints
		Filesystems    []Filesystem
		Disks          []Disk
		Raid           []Raid
		VolumeGroups   []VolumeGroup
		LogicalVolumes []LogicalVolume
	}
	tests := []struct {
		id      string
		name    string
		fields  fields
		wantErr error
	}{
		{
			id:   "fsl-1",
			name: "valid layout",
			fields: fields{
				Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large"}, Images: map[string]string{"ubuntu": "*"}},
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/sda1", Format: EXT4}, {Path: strPtr("/tmp"), Device: "tmpfs", Format: TMPFS}},
				Disks:       []Disk{{Device: "/dev/sda", Partitions: []DiskPartition{{Number: 1}}}},
			},
			wantErr: nil,
		},
		{
			id:   "fsl-2",
			name: "invalid layout, wildcard image",
			fields: fields{
				Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large"}, Images: map[string]string{"*": ""}},
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/sda1", Format: VFAT}},
				Disks:       []Disk{{Device: "/dev/sda", Partitions: []DiskPartition{{Number: 1}}}},
			},
			wantErr: errorutil.InvalidArgument("just '*' is not allowed as image os constraint"),
		},
		{
			id:   "fsl-3",
			name: "invalid layout, wildcard size",
			fields: fields{
				Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large*"}, Images: map[string]string{"debian": "*"}},
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/sda1", Format: VFAT}},
				Disks:       []Disk{{Device: "/dev/sda", Partitions: []DiskPartition{{Number: 1}}}},
			},
			wantErr: errorutil.InvalidArgument("no wildcard allowed in size constraint"),
		},
		{
			id:   "fsl-4",
			name: "invalid layout, duplicate size",
			fields: fields{
				Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large", "c1-large", "c1-xlarge"}, Images: map[string]string{"debian": "*"}},
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/sda1", Format: VFAT}},
				Disks:       []Disk{{Device: "/dev/sda", Partitions: []DiskPartition{{Number: 1}}}},
			},
			wantErr: errorutil.InvalidArgument("size c1-large is configured more than once"),
		},
		{
			id:   "fsl-5",
			name: "invalid layout /dev/sda2 is missing",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/sda1", Format: VFAT}, {Path: strPtr("/"), Device: "/dev/sda2", Format: EXT4}},
				Disks:       []Disk{{Device: "/dev/sda", Partitions: []DiskPartition{{Number: 1}}}},
			},
			wantErr: errorutil.InvalidArgument("device:/dev/sda2 for filesystem:/ is not configured"),
		},
		{
			id:   "fsl-6",
			name: "invalid layout wrong Format",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/sda1", Format: "xfs"}},
				Disks:       []Disk{{Device: "/dev/sda", Partitions: []DiskPartition{{Number: 1}}}},
			},
			wantErr: errorutil.InvalidArgument("filesystem:/boot format:xfs is not supported"),
		},
		{
			id:   "fsl-7",
			name: "invalid layout wrong GPTType",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/sda1", Format: "vfat"}},
				Disks:       []Disk{{Device: "/dev/sda", Partitions: []DiskPartition{{Number: 1, GPTType: &GPTInvalid}}}},
			},
			wantErr: errorutil.InvalidArgument("given GPTType:ff00 for partition:1 on disk:/dev/sda is not supported"),
		},
		{
			id:   "fsl-8",
			name: "valid raid layout",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/md1", Format: VFAT}},
				Disks: []Disk{
					{Device: "/dev/sda", Partitions: []DiskPartition{{Number: 1}}},
					{Device: "/dev/sdb", Partitions: []DiskPartition{{Number: 1}}},
				},
				Raid: []Raid{
					{ArrayName: "/dev/md1", Devices: []string{"/dev/sda1", "/dev/sdb1"}, Level: RaidLevel1},
				},
			},
			wantErr: nil,
		},
		{
			id:   "fsl-9",
			name: "invalid raid layout wrong level",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/md1", Format: VFAT}},
				Disks: []Disk{
					{Device: "/dev/sda", Partitions: []DiskPartition{{Number: 1}}},
					{Device: "/dev/sdb", Partitions: []DiskPartition{{Number: 1}}},
				},
				Raid: []Raid{
					{ArrayName: "/dev/md1", Devices: []string{"/dev/sda1", "/dev/sdb1"}, Level: "6"},
				},
			},
			wantErr: errorutil.InvalidArgument("given raidlevel:6 is not supported"),
		},
		{
			id:   "fsl-10",
			name: "invalid layout raid device missing",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/md1"}},
				Disks: []Disk{
					{Device: "/dev/sda", Partitions: []DiskPartition{{Number: 1}}},
					{Device: "/dev/sdb", Partitions: []DiskPartition{{Number: 1}}},
				},
			},
			wantErr: errorutil.InvalidArgument("device:/dev/md1 for filesystem:/boot is not configured"),
		},
		{
			id:   "fsl-11",
			name: "invalid layout device of raid missing",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/md1"}},
				Disks: []Disk{
					{Device: "/dev/sda", Partitions: []DiskPartition{{Number: 1}}},
					{Device: "/dev/sdb", Partitions: []DiskPartition{{Number: 1}}},
				},
				Raid: []Raid{
					{ArrayName: "/dev/md1", Devices: []string{"/dev/sda2", "/dev/sdb2"}, Level: RaidLevel1},
				},
			},
			wantErr: errorutil.InvalidArgument("device:/dev/sda2 not provided by disk for raid:/dev/md1"),
		},
		{
			id:   "fsl-12",
			name: "valid lvm layout",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/vgroot/boot", Format: VFAT}},
				Disks: []Disk{
					{Device: "/dev/sda"},
					{Device: "/dev/sdb"},
				},
				VolumeGroups: []VolumeGroup{
					{Name: "vgroot", Devices: []string{"/dev/sda", "/dev/sdb"}},
				},
				LogicalVolumes: []LogicalVolume{
					{Name: "boot", VolumeGroup: "vgroot", Size: 100000000, LVMType: LVMTypeRaid1},
				},
			},
			wantErr: nil,
		},
		{
			id:   "fsl-13",
			name: "valid lvm layout, variable size",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/vgroot/boot", Format: VFAT}, {Path: strPtr("/var"), Device: "/dev/vgroot/var", Format: EXT4}},
				Disks: []Disk{
					{Device: "/dev/sda"},
					{Device: "/dev/sdb"},
				},
				VolumeGroups: []VolumeGroup{
					{Name: "vgroot", Devices: []string{"/dev/sda", "/dev/sdb"}},
				},
				LogicalVolumes: []LogicalVolume{
					{Name: "boot", VolumeGroup: "vgroot", Size: 100000000, LVMType: LVMTypeRaid1},
					{Name: "var", VolumeGroup: "vgroot", Size: 0, LVMType: LVMTypeRaid1},
				},
			},
			wantErr: nil,
		},
		{
			id:   "fsl-14",
			name: "invalid lvm layout",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/vg00/boot", Format: VFAT}},
				Disks: []Disk{
					{Device: "/dev/sda"},
					{Device: "/dev/sdb"},
				},
				VolumeGroups: []VolumeGroup{
					{Name: "vgroot", Devices: []string{"/dev/sda", "/dev/sdb"}},
				},
				LogicalVolumes: []LogicalVolume{
					{Name: "boot", VolumeGroup: "vgroot", Size: 100, LVMType: LVMTypeRaid1},
				},
			},
			wantErr: errorutil.InvalidArgument("device:/dev/vg00/boot for filesystem:/boot is not configured"),
		},
		{
			id:   "fsl-15",
			name: "invalid lvm layout, variable size not the last one",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/vgroot/boot", Format: VFAT}},
				Disks: []Disk{
					{Device: "/dev/sda"},
					{Device: "/dev/sdb"},
				},
				VolumeGroups: []VolumeGroup{
					{Name: "vgroot", Devices: []string{"/dev/sda", "/dev/sdb"}},
				},
				LogicalVolumes: []LogicalVolume{
					{Name: "boot", VolumeGroup: "vgroot", Size: 100000000, LVMType: LVMTypeRaid1},
					{Name: "/var", VolumeGroup: "vgroot", Size: 0, LVMType: LVMTypeRaid1},
					{Name: "/opt", VolumeGroup: "vgroot", Size: 20000000, LVMType: LVMTypeRaid1},
				},
			},
			wantErr: errorutil.InvalidArgument("lv:/var in vg:vgroot, variable sized lv must be the last"),
		},
		{
			id:   "fsl-16",
			name: "invalid lvm layout, raid is configured but only one device",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/vgroot/boot", Format: VFAT}},
				Disks: []Disk{
					{Device: "/dev/sda"},
					{Device: "/dev/sdb"},
				},
				VolumeGroups: []VolumeGroup{
					{Name: "vgroot", Devices: []string{"/dev/sda"}},
				},
				LogicalVolumes: []LogicalVolume{
					{Name: "boot", VolumeGroup: "vgroot", Size: 100000000, LVMType: LVMTypeRaid1},
				},
			},
			wantErr: errorutil.InvalidArgument("fsl:\"fsl-16\" lv:boot in vg:vgroot is configured for lvmtype:raid1 but has only 1 disk, consider linear instead"),
		},
		{
			id:   "fsl-17",
			name: "invalid lvm layout, stripe is configured but only one device",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/vgroot/boot", Format: VFAT}},
				Disks: []Disk{
					{Device: "/dev/sda"},
					{Device: "/dev/sdb"},
				},
				VolumeGroups: []VolumeGroup{
					{Name: "vgroot", Devices: []string{"/dev/sda"}},
				},
				LogicalVolumes: []LogicalVolume{
					{Name: "boot", VolumeGroup: "vgroot", Size: 100000000, LVMType: LVMTypeStriped},
				},
			},
			wantErr: errorutil.InvalidArgument("fsl:\"fsl-17\" lv:boot in vg:vgroot is configured for lvmtype:striped but has only 1 disk, consider linear instead"),
		},
		{
			id:   "fsl-18",
			name: "valid lvm layout, linear is configured but only one device",
			fields: fields{
				Filesystems: []Filesystem{{Path: strPtr("/boot"), Device: "/dev/vgroot/boot", Format: VFAT}},
				Disks: []Disk{
					{Device: "/dev/sda"},
					{Device: "/dev/sdb"},
				},
				VolumeGroups: []VolumeGroup{
					{Name: "vgroot", Devices: []string{"/dev/sda"}},
				},
				LogicalVolumes: []LogicalVolume{
					{Name: "boot", VolumeGroup: "vgroot", Size: 100000000, LVMType: LVMTypeLinear},
				},
			},
			wantErr: nil,
		},
		{
			id:   "fsl-19",
			name: "invalid createoptions",
			fields: fields{
				Filesystems: []Filesystem{
					{
						Path:   strPtr("/boot/efi"),
						Device: "/dev/sda1",
						Format: VFAT,
						CreateOptions: []string{
							"-F 32",
						},
					},
				},
				Disks: []Disk{
					{Device: "/dev/sda1"},
				},
			},
			wantErr: errorutil.InvalidArgument("the given createoption:\"-F 32\" contains whitespace and must be split into separate options"),
		},
		{
			id:   "fsl-20",
			name: "valid createoptions",
			fields: fields{
				Filesystems: []Filesystem{
					{
						Path:   strPtr("/boot/efi"),
						Device: "/dev/sda1",
						Format: VFAT,
						CreateOptions: []string{
							"-F",
							"32",
						},
					},
				},
				Disks: []Disk{
					{Device: "/dev/sda1"},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			f := &FilesystemLayout{
				Base:           Base{ID: tt.id},
				Constraints:    tt.fields.Constraints,
				Filesystems:    tt.fields.Filesystems,
				Disks:          tt.fields.Disks,
				Raid:           tt.fields.Raid,
				VolumeGroups:   tt.fields.VolumeGroups,
				LogicalVolumes: tt.fields.LogicalVolumes,
			}
			err := f.Validate()
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}

func TestDisk_validate(t *testing.T) {
	type fields struct {
		Device          string
		PartitionPrefix string
		Partitions      []DiskPartition
		Wipe            bool
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr error
	}{
		{
			name:    "simple",
			fields:  fields{Device: "/dev/sda", PartitionPrefix: "/dev/sda", Partitions: []DiskPartition{{Number: 1}}},
			wantErr: nil,
		},
		{
			name: "fails because not last partition is variable",
			fields: fields{
				Device: "/dev/sda", PartitionPrefix: "/dev/sda",
				Partitions: []DiskPartition{
					{Number: 1, Size: 100},
					{Number: 2, Size: 0},
					{Number: 3, Size: 100},
				}},
			wantErr: errorutil.InvalidArgument("device:/dev/sda variable sized partition not the last one"),
		},
		{
			name: "fails because not duplicate partition number",
			fields: fields{
				Device: "/dev/sda", PartitionPrefix: "/dev/sda",
				Partitions: []DiskPartition{
					{Number: 1, Size: 100},
					{Number: 2, Size: 100},
					{Number: 2, Size: 100},
				}},
			wantErr: errorutil.InvalidArgument("device:/dev/sda partition number:2 given more than once"),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			d := Disk{
				Device:          tt.fields.Device,
				Partitions:      tt.fields.Partitions,
				WipeOnReinstall: tt.fields.Wipe,
			}
			err := d.validate()
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}

func TestFilesystemLayouts_Validate(t *testing.T) {
	tests := []struct {
		name    string
		fls     FilesystemLayouts
		wantErr error
	}{
		{
			name: "simple valid",
			fls: FilesystemLayouts{
				{Base: Base{ID: "default"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large", "c1-xlarge"}, Images: map[string]string{"ubuntu": "*", "debian": "*"}}},
				{Base: Base{ID: "firewall"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large", "c1-xlarge"}, Images: map[string]string{"firewall": "*"}}},
			},
			wantErr: nil,
		},
		{
			name: "valid with open layout",
			fls: FilesystemLayouts{
				{Base: Base{ID: "default"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large", "c1-xlarge"}, Images: map[string]string{"ubuntu": "*", "debian": "*"}}},
				{Base: Base{ID: "develop-1"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{}, Images: map[string]string{}}},
				{Base: Base{ID: "develop-2"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{}, Images: map[string]string{}}},
				{Base: Base{ID: "firewall"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large", "c1-xlarge"}, Images: map[string]string{"firewall": "*"}}},
			},
			wantErr: nil,
		},
		{
			name: "simple not overlapping, different sizes, same images",
			fls: FilesystemLayouts{
				{Base: Base{ID: "default"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large", "c1-xlarge"}, Images: map[string]string{"ubuntu": "*", "debian": "*"}}},
				{Base: Base{ID: "default2"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{"s1-large", "s1-xlarge"}, Images: map[string]string{"ubuntu": "*", "debian": "*"}}},
				{Base: Base{ID: "firewall"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large", "c1-xlarge"}, Images: map[string]string{"firewall": "*"}}},
			},
			wantErr: nil,
		},
		{
			name: "one overlapping, different sizes, same images",
			fls: FilesystemLayouts{
				{Base: Base{ID: "default"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large", "c1-xlarge"}, Images: map[string]string{"ubuntu": "*", "debian": ">= 10"}}},
				{Base: Base{ID: "default2"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large", "s1-large", "s1-xlarge"}, Images: map[string]string{"ubuntu": "*", "debian": "< 9"}}},
				{Base: Base{ID: "firewall"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large", "c1-xlarge"}, Images: map[string]string{"firewall": "*"}}},
			},
			wantErr: errorutil.InvalidArgument("these combinations already exist:c1-large->[ubuntu *]"),
		},
		{
			name: "one overlapping, same sizes, different images",
			// FIXME fails
			fls: FilesystemLayouts{
				{Base: Base{ID: "default"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large"}, Images: map[string]string{"debian": ">= 10"}}},
				{Base: Base{ID: "default2"}, Constraints: FilesystemLayoutConstraints{Sizes: []string{"c1-large"}, Images: map[string]string{"debian": "< 10"}}},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fls.Validate()
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}

func Test_convertToOpAndVersion(t *testing.T) {
	tests := []struct {
		name              string
		versionconstraint string
		op                string
		version           *semver.Version
		wantErr           error
	}{
		{
			name:              "simple",
			versionconstraint: ">= 10.0.1",
			op:                ">=",
			version:           semver.MustParse("10.0.1"),
			wantErr:           nil,
		},
		{
			name:              "invalid no space",
			versionconstraint: ">=10.0.1",
			op:                "",
			version:           nil,
			wantErr:           errorutil.InvalidArgument("given imageconstraint:>=10.0.1 is not valid, missing space between op and version? invalid semantic version"),
		},
		{
			name:              "invalid version",
			versionconstraint: ">= 10.x.1",
			op:                "",
			version:           nil,
			wantErr:           errorutil.InvalidArgument("given version:10.x.1 is not valid:invalid semantic version"),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := convertToOpAndVersion(tt.versionconstraint)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if got != tt.op {
				t.Errorf("convertToOpAndVersion() got = %v, want %v", got, tt.op)
			}
			if !reflect.DeepEqual(got1, tt.version) {
				t.Errorf("convertToOpAndVersion() got1 = %v, want %v", got1, tt.version)
			}
		})
	}
}

func Test_hasCollisions(t *testing.T) {
	tests := []struct {
		name               string
		versionConstraints []string
		wantErr            error
	}{
		{
			name:               "simple",
			versionConstraints: []string{">= 10", "<= 9.9"},
			wantErr:            nil,
		},
		{
			name:               "simple 2",
			versionConstraints: []string{">= 10", "< 10"},
			wantErr:            nil,
		},
		{
			name:               "simple star match",
			versionConstraints: []string{">= 10", "<= 9.9", "*"},
			wantErr:            errorutil.InvalidArgument("at least one `*` and more than one constraint"),
		},
		{
			name:               "simple versions overlap",
			versionConstraints: []string{">= 10", "<= 9.9", ">= 9.8"},
			wantErr:            errorutil.InvalidArgument("constraint:<=9.9 overlaps:>=9.8"),
		},
		{
			name:               "simple versions overlap reverse",
			versionConstraints: []string{">= 9.8", "<= 9.9", ">= 10"},
			wantErr:            errorutil.InvalidArgument("constraint:>=9.8 overlaps:<=9.9"),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := hasCollisions(tt.versionConstraints)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}

func TestToFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		want    Format
		wantErr error
	}{
		{
			name:    "valid format",
			format:  "ext4",
			want:    EXT4,
			wantErr: nil,
		},
		{
			name:    "invalid format",
			format:  "ext5",
			wantErr: errorutil.InvalidArgument("given format:ext5 is not supported, but:ext3,ext4,none,swap,tmpfs,vfat"),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToFormat(tt.format)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if got != nil && *got != tt.want {
				t.Errorf("ToFormat() = %v, want %v", string(*got), tt.want)
			}
		})
	}
}

func TestToGPTType(t *testing.T) {
	tests := []struct {
		name    string
		gpttyp  string
		want    GPTType
		wantErr error
	}{
		{
			name:    "valid type",
			gpttyp:  "8300",
			want:    GPTLinux,
			wantErr: nil,
		},
		{
			name:    "invalid type",
			gpttyp:  "8301",
			wantErr: errorutil.InvalidArgument("given GPTType:8301 is not supported, but:8300,8e00,ef00,fd00"),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToGPTType(tt.gpttyp)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if got != nil && *got != tt.want {
				t.Errorf("ToGPTType() = %v, want %v", string(*got), tt.want)
			}
		})
	}
}

func TestToRaidLevel(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		want    RaidLevel
		wantErr error
	}{
		{
			name:    "valid level",
			level:   "1",
			want:    RaidLevel1,
			wantErr: nil,
		},
		{
			name:    "invalid level",
			level:   "raid5",
			wantErr: errorutil.InvalidArgument("given raidlevel:raid5 is not supported, but:0,1"),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToRaidLevel(tt.level)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if got != nil && *got != tt.want {
				t.Errorf("ToRaidLevel() = %v, want %v", string(*got), tt.want)
			}
		})
	}
}

func TestToLVMType(t *testing.T) {
	tests := []struct {
		name    string
		lvmtyp  string
		want    LVMType
		wantErr error
	}{
		{
			name:    "valid lvmtype",
			lvmtyp:  "linear",
			want:    LVMTypeLinear,
			wantErr: nil,
		},
		{
			name:    "invalid lvmtype",
			lvmtyp:  "raid5",
			wantErr: errorutil.InvalidArgument("given lvmtype:raid5 is not supported, but:linear,raid1,striped"),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToLVMType(tt.lvmtyp)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
			if got != nil && *got != tt.want {
				t.Errorf("ToLVMType() = %v, want %v", string(*got), tt.want)
			}
		})
	}
}
