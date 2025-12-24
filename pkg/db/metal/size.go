package metal

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/dustin/go-humanize"
	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/samber/lo"
)

// A Size represents a supported machine size.
type Size struct {
	Base
	Constraints []Constraint      `rethinkdb:"constraints"`
	Labels      map[string]string `rethinkdb:"labels"`
}

// Sizes is a list of sizes.
type Sizes []Size

// ConstraintType ...
type ConstraintType string

// come constraint types
const (
	CoreConstraint    ConstraintType = "cores"
	MemoryConstraint  ConstraintType = "memory"
	StorageConstraint ConstraintType = "storage"
	GPUConstraint     ConstraintType = "gpu"
)

var allConstraintTypes = []ConstraintType{CoreConstraint, MemoryConstraint, StorageConstraint, GPUConstraint}

// A Constraint describes the hardware constraints for a given size.
type Constraint struct {
	Type       ConstraintType `rethinkdb:"type"`
	Min        uint64         `rethinkdb:"min"`
	Max        uint64         `rethinkdb:"max"`
	Identifier string         `rethinkdb:"identifier" description:"glob of the identifier of this type"`
}

func FromConstraint(c Constraint) (*apiv2.SizeConstraint, error) {
	apiv2SizeConstraintType, err := enum.GetEnum[apiv2.SizeConstraintType](string(c.Type))
	if err != nil {
		return nil, err
	}
	return &apiv2.SizeConstraint{
		Type:       apiv2SizeConstraintType,
		Min:        c.Min,
		Max:        c.Max,
		Identifier: pointer.PointerOrNil(c.Identifier),
	}, nil
}

func ToConstraint(c *apiv2.SizeConstraint) (*Constraint, error) {
	var constraintType ConstraintType
	switch c.Type {
	case apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES:
		constraintType = CoreConstraint
	case apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_GPU:
		constraintType = GPUConstraint
	case apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY:
		constraintType = MemoryConstraint
	case apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE:
		constraintType = StorageConstraint
	case apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_UNSPECIFIED:
		fallthrough
	default:
		return nil, fmt.Errorf("given constraint has unknown type %q", c.Type)
	}

	return &Constraint{
		Type:       constraintType,
		Min:        c.Min,
		Max:        c.Max,
		Identifier: pointer.SafeDeref(c.Identifier),
	}, nil
}

// Overlaps returns nil if Size does not overlap with any other size, otherwise returns overlapping Size
func (s *Size) Overlaps(ss Sizes) *Size {
	for _, so := range ss {
		if s.ID == so.ID {
			continue
		}
		if s.overlaps(&so) {
			return &so
		}
	}
	return nil
}

func (s *Size) overlaps(so *Size) bool {
	if len(pointer.SafeDeref(so).Constraints) == 0 || len(pointer.SafeDeref(s).Constraints) == 0 {
		// If no constraints are present, this size will overlap with all other sizes
		return true
	}

	srcTypes := lo.GroupBy(s.Constraints, func(item Constraint) ConstraintType {
		return item.Type
	})
	destTypes := lo.GroupBy(so.Constraints, func(item Constraint) ConstraintType {
		return item.Type
	})

	for t, srcConstraints := range srcTypes {
		destConstraints, ok := destTypes[t]
		if !ok {
			// Strictly speaking this is wrong, but machines might have no gpus
			// We should prevent sizes without cpu/memory/storage constraints
			return false
		}
		for _, sc := range srcConstraints {
			for _, dc := range destConstraints {
				if !dc.overlaps(sc) {
					return false
				}
			}
		}
	}

	for t, destConstraints := range destTypes {
		srcConstraints, ok := srcTypes[t]
		if !ok {
			// Strictly speaking this is wrong, but machines might have no gpus
			// We should prevent sizes without cpu/memory/storage constraints
			return false
		}
		for _, dc := range destConstraints {
			for _, sc := range srcConstraints {
				if !sc.overlaps(dc) {
					return false
				}
			}
		}
	}

	return true
}

// overlaps is correct under the precondition that max is not smaller than min
func (c *Constraint) overlaps(other Constraint) bool {
	if c.Type != other.Type {
		return false
	}

	if c.Identifier != other.Identifier {
		return false
	}

	if c.Min > other.Max {
		return false
	}

	if c.Max < other.Min {
		return false
	}

	return true
}

func (c *Constraint) Validate() error {
	if c.Max < c.Min {
		return fmt.Errorf("max is smaller than min")
	}

	if _, err := filepath.Match(c.Identifier, ""); err != nil {
		return fmt.Errorf("identifier is malformed: %w", err)
	}

	switch c.Type {
	case GPUConstraint:
		if c.Identifier == "" {
			return fmt.Errorf("for gpu constraints an identifier is required")
		}
	case MemoryConstraint:
		if c.Identifier != "" {
			return fmt.Errorf("for memory constraints an identifier is not allowed")
		}
	case CoreConstraint, StorageConstraint:
	}

	return nil
}


// FromHardware searches a Size for given hardware specs. It will search
// for a size where the constraints matches the given hardware.
func (sz Sizes) FromHardware(hardware MachineHardware) (*Size, error) {
	var (
		matchedSizes []Size
	)

nextsize:
	for _, s := range sz {
		for _, c := range s.Constraints {
			if !c.matches(hardware) {
				continue nextsize
			}
		}

		for _, ct := range allConstraintTypes {
			if !hardware.matches(s.Constraints, ct) {
				continue nextsize
			}
		}

		matchedSizes = append(matchedSizes, s)
	}

	switch len(matchedSizes) {
	case 0:
		return nil, errorutil.NotFound("no size found for hardware (%s)", hardware.ReadableSpec())
	case 1:
		return &matchedSizes[0], nil
	default:
		return nil, fmt.Errorf("%d sizes found for hardware (%s)", len(matchedSizes), hardware.ReadableSpec())
	}
}

func (c *Constraint) inRange(value uint64) bool {
	return value >= c.Min && value <= c.Max
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

// matches returns true if the given machine hardware is inside the min/max values of the
// constraint.
func (c *Constraint) matches(hw MachineHardware) bool {
	res := false
	switch c.Type {
	case CoreConstraint:
		cores, _ := capacityOf(c.Identifier, hw.MetalCPUs, countCPU)
		res = c.inRange(cores)
	case MemoryConstraint:
		res = c.inRange(hw.Memory)
	case StorageConstraint:
		capacity, _ := capacityOf(c.Identifier, hw.Disks, countDisk)
		res = c.inRange(capacity)
	case GPUConstraint:
		count, _ := capacityOf(c.Identifier, hw.MetalGPUs, countGPU)
		res = c.inRange(count)
	}
	return res
}

// matches returns true if all provided disks and later GPUs are covered with at least one constraint.
// With this we ensure that hardware matches exhaustive against the constraints.
func (hw *MachineHardware) matches(constraints []Constraint, constraintType ConstraintType) bool {
	filtered := lo.Filter(constraints, func(c Constraint, _ int) bool { return c.Type == constraintType })

	switch constraintType {
	case StorageConstraint:
		return exhaustiveMatch(filtered, hw.Disks, countDisk)
	case GPUConstraint:
		return exhaustiveMatch(filtered, hw.MetalGPUs, countGPU)
	case CoreConstraint:
		return exhaustiveMatch(filtered, hw.MetalCPUs, countCPU)
	case MemoryConstraint:
		return exhaustiveMatch(filtered, []uint64{hw.Memory}, countMemory)
	default:
		return false
	}
}

// ReadableSpec returns a human readable string for the hardware.
func (hw *MachineHardware) ReadableSpec() string {
	diskCapacity, _ := capacityOf("*", hw.Disks, countDisk)
	cpus, _ := capacityOf("*", hw.MetalCPUs, countCPU)
	gpus, _ := capacityOf("*", hw.MetalGPUs, countGPU)
	return fmt.Sprintf("CPUs: %d, Memory: %s, Storage: %s, GPUs: %d", cpus, humanize.Bytes(hw.Memory), humanize.Bytes(diskCapacity), gpus)
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