package metal

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/dustin/go-humanize"
	"github.com/samber/lo"
)

// MachineHardware stores the data which is collected by our system on the hardware when it registers itself.
type MachineHardware struct {
	Memory    uint64        `rethinkdb:"memory" json:"memory"`
	Nics      Nics          `rethinkdb:"network_interfaces" json:"network_interfaces"`
	Disks     []BlockDevice `rethinkdb:"block_devices" json:"block_devices"`
	MetalCPUs []MetalCPU    `rethinkdb:"cpus" json:"cpus"`
	MetalGPUs []MetalGPU    `rethinkdb:"gpus" json:"gpus"`
}

type MetalCPU struct {
	Vendor  string `rethinkdb:"vendor" json:"vendor"`
	Model   string `rethinkdb:"model" json:"model"`
	Cores   uint32 `rethinkdb:"cores" json:"cores"`
	Threads uint32 `rethinkdb:"threads" json:"threads"`
}

type MetalGPU struct {
	Vendor string `rethinkdb:"vendor" json:"vendor"`
	Model  string `rethinkdb:"model" json:"model"`
}

func countCPU(cpu MetalCPU) (model string, count uint64) {
	return cpu.Model, uint64(cpu.Cores)
}

func countGPU(gpu MetalGPU) (model string, count uint64) {
	return gpu.Model, 1
}

func countDisk(disk BlockDevice) (model string, count uint64) {
	return disk.Name, disk.Size
}

func countMemory(size uint64) (model string, count uint64) {
	return "", size
}

func capacityOf[V any](identifier string, vs []V, countFn func(v V) (model string, count uint64)) (uint64, []V) {
	var (
		sum     uint64
		matched []V
	)

	for _, v := range vs {
		model, count := countFn(v)

		if identifier != "" {
			matches, err := filepath.Match(identifier, model)
			if err != nil {
				// illegal identifiers are already prevented by size validation
				continue
			}

			if !matches {
				continue
			}
		}

		sum += count
		matched = append(matched, v)
	}

	return sum, matched
}

func exhaustiveMatch[V comparable](cs []Constraint, vs []V, countFn func(v V) (model string, count uint64)) bool {
	unmatched := slices.Clone(vs)

	for _, c := range cs {
		capacity, matched := capacityOf(c.Identifier, vs, countFn)

		match := c.inRange(capacity)
		if !match {
			continue
		}

		unmatched, _ = lo.Difference(unmatched, matched)
	}

	return len(unmatched) == 0
}

// ReadableSpec returns a human readable string for the hardware.
func (hw *MachineHardware) ReadableSpec() string {
	diskCapacity, _ := capacityOf("*", hw.Disks, countDisk)
	cpus, _ := capacityOf("*", hw.MetalCPUs, countCPU)
	gpus, _ := capacityOf("*", hw.MetalGPUs, countGPU)
	return fmt.Sprintf("CPUs: %d, Memory: %s, Storage: %s, GPUs: %d", cpus, humanize.Bytes(hw.Memory), humanize.Bytes(diskCapacity), gpus)
}

// BlockDevice information.
type BlockDevice struct {
	Name string `rethinkdb:"name" json:"name"`
	Size uint64 `rethinkdb:"size" json:"size"`
}
