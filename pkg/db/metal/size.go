package metal

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
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
		constraints, ok := destTypes[t]
		if !ok {
			return false
		}
		for _, sc := range srcConstraints {
			for _, c := range constraints {
				if !c.overlaps(sc) {
					return false
				}
			}
		}
	}

	for t, destConstraints := range destTypes {
		constraints, ok := srcTypes[t]
		if !ok {
			return false
		}
		for _, sc := range destConstraints {
			for _, c := range constraints {
				if !c.overlaps(sc) {
					return false
				}
			}
		}
	}

	return true
}

// overlaps is proven correct, requires that constraint are validated before that max is not smaller than min
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

// Validate a size, returns error if a invalid size is passed
func (s *Size) Validate(partitions PartitionMap) error {
	var (
		errs       []error
		typeCounts = map[ConstraintType]uint{}
	)

	for i, c := range s.Constraints {
		typeCounts[c.Type]++

		err := c.validate()
		if err != nil {
			errs = append(errs, fmt.Errorf("constraint at index %d is invalid: %w", i, err))
		}

		switch t := c.Type; t {
		case GPUConstraint, StorageConstraint:
		case MemoryConstraint, CoreConstraint:
			if typeCounts[t] > 1 {
				errs = append(errs, fmt.Errorf("constraint at index %d is invalid: type duplicates are not allowed for type %q", i, t))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("size %q is invalid: %w", s.ID, errors.Join(errs...))
	}

	return nil
}

func (c *Constraint) validate() error {
	if c.Max < c.Min {
		return fmt.Errorf("max is smaller than min")
	}

	if _, err := filepath.Match(c.Identifier, ""); err != nil {
		return fmt.Errorf("identifier is malformed: %w", err)
	}

	switch t := c.Type; t {
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
