package metal

const (
	// VFAT is used for the UEFI boot partition
	VFAT = Format("vfat")
	// EXT3 is usually only used for /boot
	EXT3 = Format("ext3")
	// EXT4 is the default fs
	EXT4 = Format("ext4")
	// SWAP is for the swap partition
	SWAP = Format("swap")
	// TMPFS is used for a memory filesystem typically /tmp
	TMPFS = Format("tmpfs")
	// None
	NONE = Format("none")

	// GPTBoot EFI Boot Partition
	GPTBoot = GPTType("ef00")
	// GPTLinux Linux Partition
	GPTLinux = GPTType("8300")
	// GPTLinuxRaid Linux Raid Partition
	GPTLinuxRaid = GPTType("fd00")
	// GPTLinux Linux Partition
	GPTLinuxLVM = GPTType("8e00")

	// RaidLevel0 is a stripe of two or more disks
	RaidLevel0 = RaidLevel("0")
	// RaidLevel1 is a mirror of two disks
	RaidLevel1 = RaidLevel("1")

	// LVMTypeLinear append across all physical volumes
	LVMTypeLinear = LVMType("linear")
	// LVMTypeStriped stripe across all physical volumes
	LVMTypeStriped = LVMType("striped")
	// LVMTypeStripe mirror with raid across all physical volumes
	LVMTypeRaid1 = LVMType("raid1")
)

type (
	// FilesystemLayouts is a slice of FilesystemLayout
	FilesystemLayouts []FilesystemLayout
	// FilesystemLayout to be created on the given machine
	FilesystemLayout struct {
		Base
		// Filesystems to create on the server
		Filesystems []Filesystem `rethinkdb:"filesystems"`
		// Disks to configure in the server with their partitions
		Disks []Disk `rethinkdb:"disks"`
		// Raid if not empty, create raid arrays out of the individual disks, to place filesystems onto
		Raid []Raid `rethinkdb:"raid"`
		// VolumeGroups to create
		VolumeGroups []VolumeGroup `rethinkdb:"volumegroups"`
		// LogicalVolumes to create on top of VolumeGroups
		LogicalVolumes LogicalVolumes `rethinkdb:"logicalvolumes"`
		// Constraints which must match to select this Layout
		Constraints FilesystemLayoutConstraints `rethinkdb:"constraints"`
	}

	// LogicalVolumes is a slice of LogicalVolume
	LogicalVolumes []LogicalVolume

	FilesystemLayoutConstraints struct {
		// Sizes defines the list of sizes this layout applies to
		Sizes []string `rethinkdb:"sizes"`
		// Images defines a map from os to versionconstraint
		// the combination of os and versionconstraint per size must be conflict free over all filesystemlayouts
		Images map[string]string `rethinkdb:"images"`
	}

	RaidLevel string
	Format    string
	GPTType   string
	LVMType   string

	// Filesystem defines a single filesystem to be mounted
	Filesystem struct {
		// Path defines the mountpoint, if nil, it will not be mounted
		Path *string `rethinkdb:"path"`
		// Device where the filesystem is created on, must be the full device path seen by the OS
		Device string `rethinkdb:"device"`
		// Format is the type of filesystem should be created
		Format Format `rethinkdb:"format"`
		// Label is optional enhances readability
		Label *string `rethinkdb:"label"`
		// MountOptions which might be required
		MountOptions []string `rethinkdb:"mountoptions"`
		// CreateOptions during filesystem creation
		CreateOptions []string `rethinkdb:"createoptions"`
	}

	// Disk represents a single block device visible from the OS, required
	Disk struct {
		// Device is the full device path
		Device string `rethinkdb:"device"`
		// Partitions to create on this device
		Partitions []DiskPartition `rethinkdb:"partitions"`
		// WipeOnReinstall, if set to true the whole disk will be erased if reinstall happens
		// during fresh install all disks are wiped
		WipeOnReinstall bool `rethinkdb:"wipeonreinstall"`
	}

	// Raid is optional, if given the devices must match.
	Raid struct {
		// ArrayName of the raid device, most often this will be /dev/md0 and so forth
		ArrayName string `rethinkdb:"arrayname"`
		// Devices the devices to form a raid device
		Devices []string `rethinkdb:"devices"`
		// Level the raidlevel to use, can be one of 0,1
		Level RaidLevel `rethinkdb:"raidlevel"`
		// CreateOptions required during raid creation, example: --metadata=1.0 for uefi boot partition
		CreateOptions []string `rethinkdb:"createoptions"`
		// Spares defaults to 0
		Spares int `rethinkdb:"spares"`
	}

	// VolumeGroup is optional, if given the devices must match.
	VolumeGroup struct {
		// Name of the volumegroup without the /dev prefix
		Name string `rethinkdb:"name"`
		// Devices the devices to form a volumegroup device
		Devices []string `rethinkdb:"devices"`
		// Tags to attach to the volumegroup
		Tags []string `rethinkdb:"tags"`
	}

	// LogicalVolume is a block devices created with lvm on top of a volumegroup
	LogicalVolume struct {
		// Name the name of the logical volume, without /dev prefix, will be accessible at /dev/vgname/lvname
		Name string `rethinkdb:"name"`
		// VolumeGroup the name of the volumegroup
		VolumeGroup string `rethinkdb:"volumegroup"`
		// Size of this LV in mebibytes (MiB), if zero all remaining space in the vg will be used.
		Size uint64 `rethinkdb:"size"`
		// LVMType can be either linear, striped or raid1
		LVMType LVMType `rethinkdb:"lvmtype"`
	}

	// DiskPartition is a single partition on a device, only GPT partition types are supported
	DiskPartition struct {
		// Number of this partition, will be added to partitionprefix
		Number uint8 `rethinkdb:"number"`
		// Label to enhance readability
		Label *string `rethinkdb:"label"`
		// Size of this partition in mebibytes (MiB)
		// if "0" is given the rest of the device will be used, this requires Number to be the highest in this partition
		Size uint64 `rethinkdb:"size"`
		// GPTType defines the GPT partition type
		GPTType *GPTType `rethinkdb:"gpttype"`
	}
)
