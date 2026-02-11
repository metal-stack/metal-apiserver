package metal

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

var (
	microSize = Size{
		Base: Base{
			ID: "micro",
		},
		Constraints: []Constraint{
			{
				Type: CoreConstraint,
				Min:  1,
				Max:  1,
			},
			{
				Type: MemoryConstraint,
				Min:  1024,
				Max:  1024,
			},
			{
				Type: StorageConstraint,
				Min:  0,
				Max:  1024,
			},
		},
	}
	tinySize = Size{
		Base: Base{
			ID: "tiny",
		},
		Constraints: []Constraint{
			{
				Type: CoreConstraint,
				Min:  1,
				Max:  1,
			},
			{
				Type: MemoryConstraint,
				Min:  1025,
				Max:  1077838336,
			},
			{
				Type: StorageConstraint,
				Min:  1024,
				Max:  2048,
			},
		},
	}
)

func TestFromConstraint(t *testing.T) {
	tests := []struct {
		name    string
		c       Constraint
		want    *apiv2.SizeConstraint
		wantErr bool
	}{
		{
			name: "core constraint",
			c:    Constraint{Type: CoreConstraint, Min: 1, Max: 1, Identifier: "Intel"},
			want: &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 1, Max: 1, Identifier: new("Intel")},
		},
		{
			name: "memory constraint",
			c:    Constraint{Type: MemoryConstraint, Min: 1, Max: 1, Identifier: "Samsung"},
			want: &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1, Max: 1, Identifier: new("Samsung")},
		},
		{
			name: "gpu constraint",
			c:    Constraint{Type: GPUConstraint, Min: 1, Max: 1, Identifier: "NVidia"},
			want: &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_GPU, Min: 1, Max: 1, Identifier: new("NVidia")},
		},
		{
			name: "Storage constraint",
			c:    Constraint{Type: StorageConstraint, Min: 1, Max: 1, Identifier: "Kingston"},
			want: &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1, Max: 1, Identifier: new("Kingston")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromConstraint(tt.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromConstraint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromConstraint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToConstraint(t *testing.T) {
	tests := []struct {
		name    string
		c       *apiv2.SizeConstraint
		want    *Constraint
		wantErr bool
	}{
		{
			name:    "core constraint",
			c:       &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 1, Max: 1, Identifier: new("Intel")},
			want:    &Constraint{Type: CoreConstraint, Min: 1, Max: 1, Identifier: "Intel"},
			wantErr: false,
		},

		{
			name: "memory constraint",
			c:    &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1, Max: 1, Identifier: new("Samsung")},
			want: &Constraint{Type: MemoryConstraint, Min: 1, Max: 1, Identifier: "Samsung"},
		},
		{
			name: "gpu constraint",
			c:    &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_GPU, Min: 1, Max: 1, Identifier: new("NVidia")},
			want: &Constraint{Type: GPUConstraint, Min: 1, Max: 1, Identifier: "NVidia"},
		},
		{
			name: "Storage constraint",
			c:    &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1, Max: 1, Identifier: new("Kingston")},
			want: &Constraint{Type: StorageConstraint, Min: 1, Max: 1, Identifier: "Kingston"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToConstraint(tt.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToConstraint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToConstraint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSizes_Overlaps(t *testing.T) {
	tests := []struct {
		name  string
		sz    Size
		sizes Sizes
		want  *Size
	}{
		{
			name: "non-overlapping size",
			sz: Size{
				Base: Base{
					ID: "micro",
				},
				Constraints: []Constraint{
					{
						Type: CoreConstraint,
						Min:  1,
						Max:  1,
					},
					{
						Type: MemoryConstraint,
						Min:  1024,
						Max:  1024,
					},
					{
						Type: StorageConstraint,
						Min:  0,
						Max:  1024,
					},
				},
			},
			sizes: Sizes{
				tinySize,
				Size{
					Base: Base{
						ID: "large",
					},
					Constraints: []Constraint{
						{
							Type: CoreConstraint,
							Min:  8,
							Max:  16,
						},
						{
							Type: MemoryConstraint,
							Min:  1024,
							Max:  1077838336,
						},
						{
							Type: StorageConstraint,
							Min:  1024,
							Max:  2048,
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "overlapping size",
			sz: Size{
				Base: Base{
					ID: "microOverlapping",
				},
				Constraints: []Constraint{
					{
						Type: CoreConstraint,
						Min:  1,
						Max:  1,
					},
					{
						Type: MemoryConstraint,
						Min:  1024,
						Max:  1024,
					},
					{
						Type: StorageConstraint,
						Min:  0,
						Max:  2048,
					},
				},
			},
			sizes: Sizes{
				{
					Base: Base{
						ID: "micro",
					},
					Constraints: []Constraint{
						{
							Type: CoreConstraint,
							Min:  1,
							Max:  1,
						},
						{
							Type: MemoryConstraint,
							Min:  1024,
							Max:  1024,
						},
						{
							Type: StorageConstraint,
							Min:  0,
							Max:  1024,
						},
					},
				},
				{
					Base: Base{
						ID: "tiny",
					},
					Constraints: []Constraint{
						{
							Type: CoreConstraint,
							Min:  1,
							Max:  1,
						},
						{
							Type: MemoryConstraint,
							Min:  1025,
							Max:  1077838336,
						},
						{
							Type: StorageConstraint,
							Min:  1024,
							Max:  2048,
						},
					},
				},
				Size{
					Base: Base{
						ID: "large",
					},
					Constraints: []Constraint{
						{
							Type: CoreConstraint,
							Min:  8,
							Max:  16,
						},
						{
							Type: MemoryConstraint,
							Min:  1024,
							Max:  1077838336,
						},
						{
							Type: StorageConstraint,
							Min:  1024,
							Max:  2048,
						},
					},
				},
			},
			want: &microSize,
		},
		{
			name: "add incomplete size",
			sz: Size{
				Base: Base{
					ID: "microIncomplete",
				},
				Constraints: []Constraint{
					{
						Type: CoreConstraint,
						Min:  1,
						Max:  1,
					},
				},
			},
			sizes: Sizes{
				microSize,
				tinySize,
				Size{
					Base: Base{
						ID: "large",
					},
					Constraints: []Constraint{
						{
							Type: CoreConstraint,
							Min:  8,
							Max:  16,
						},
						{
							Type: MemoryConstraint,
							Min:  1024,
							Max:  1077838336,
						},
						{
							Type: StorageConstraint,
							Min:  1024,
							Max:  2048,
						},
					},
				},
			},
			want: nil,
		},

		{
			name: "two different sizes",
			sz: Size{
				Base: Base{
					ID: "two different",
				},
				Constraints: []Constraint{
					{
						Type: CoreConstraint,
						Min:  1,
						Max:  1,
					},
				},
			},
			sizes: Sizes{
				Size{
					Base: Base{
						ID: "micro",
					},
					Constraints: []Constraint{
						{
							Type: MemoryConstraint,
							Min:  1024,
							Max:  1024,
						},
					},
				},
			},
			want: nil,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sz.Overlaps(tt.sizes)

			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestConstraint_overlaps(t *testing.T) {
	tests := []struct {
		name  string
		this  Constraint
		other Constraint
		want  bool
	}{
		{
			name: "no overlap, different types",
			this: Constraint{
				Type: CoreConstraint,
			},
			other: Constraint{
				Type: GPUConstraint,
			},
			want: false,
		},
		{
			name: "no overlap, different identifiers",
			this: Constraint{
				Type:       CoreConstraint,
				Identifier: "b",
			},
			other: Constraint{
				Type:       CoreConstraint,
				Identifier: "a",
			},
			want: false,
		},

		{
			name: "no overlap, different range",
			this: Constraint{
				Type:       CoreConstraint,
				Identifier: "a",
				Min:        0,
				Max:        2,
			},
			other: Constraint{
				Type:       CoreConstraint,
				Identifier: "a",
				Min:        3,
				Max:        4,
			},
			want: false,
		},

		{
			name: "partial overlap, over range",
			this: Constraint{
				Type:       CoreConstraint,
				Identifier: "a",
				Min:        0,
				Max:        4,
			},
			other: Constraint{
				Type:       CoreConstraint,
				Identifier: "a",
				Min:        3,
				Max:        5,
			},
			want: true,
		},

		{
			name: "partial overlap, under range",
			this: Constraint{
				Type:       CoreConstraint,
				Identifier: "a",
				Min:        2,
				Max:        4,
			},
			other: Constraint{
				Type:       CoreConstraint,
				Identifier: "a",
				Min:        1,
				Max:        3,
			},
			want: true,
		},
		{
			name: "partial overlap, in range",
			this: Constraint{
				Type:       CoreConstraint,
				Identifier: "a",
				Min:        1,
				Max:        5,
			},
			other: Constraint{
				Type:       CoreConstraint,
				Identifier: "a",
				Min:        2,
				Max:        3,
			},
			want: true,
		},
		{
			name: "different disk types",
			this: Constraint{
				Type:       StorageConstraint,
				Identifier: "/dev/sd*",
				Min:        1,
				Max:        5,
			},
			other: Constraint{
				Type:       StorageConstraint,
				Identifier: "/dev/nvme*",
				Min:        1,
				Max:        5,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if got := tt.this.overlaps(tt.other); got != tt.want {
				t.Errorf("Constraint.overlaps() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSize_overlaps(t *testing.T) {
	tests := []struct {
		name        string
		this        *Size
		other       *Size
		wantOverlap bool
	}{
		{
			name: "no overlap, different types",
			this: &Size{
				Constraints: []Constraint{
					{Type: MemoryConstraint},
				},
			},
			other: &Size{
				Constraints: []Constraint{
					{Type: CoreConstraint},
				},
			},
			wantOverlap: false,
		},
		{
			name: "no overlap, different identifiers",
			this: &Size{
				Constraints: []Constraint{
					{Type: MemoryConstraint, Identifier: "a"},
				},
			},
			other: &Size{
				Constraints: []Constraint{
					{Type: MemoryConstraint, Identifier: "b"},
				},
			},
			wantOverlap: false,
		},
		{
			name: "no overlap, different range",
			this: &Size{
				Constraints: []Constraint{
					{Type: MemoryConstraint, Identifier: "a", Min: 0, Max: 4},
				},
			},
			other: &Size{
				Constraints: []Constraint{
					{Type: MemoryConstraint, Identifier: "a", Min: 5, Max: 8},
				},
			},
			wantOverlap: false,
		},
		{
			name: "no overlap, different gpus",
			this: &Size{
				Constraints: []Constraint{
					{Type: GPUConstraint, Identifier: "a", Min: 1, Max: 1},
				},
			},
			other: &Size{
				Constraints: []Constraint{
					{Type: GPUConstraint, Identifier: "a", Min: 1, Max: 1},
					{Type: GPUConstraint, Identifier: "b", Min: 2, Max: 2},
				},
			},
			wantOverlap: false,
		},
		{
			name: "overlapping size",
			this: &Size{
				Constraints: []Constraint{
					{
						Type: CoreConstraint,
						Min:  1,
						Max:  1,
					},
					{
						Type: MemoryConstraint,
						Min:  1024,
						Max:  1024,
					},
					{
						Type: StorageConstraint,
						Min:  0,
						Max:  2048,
					},
				},
			},
			other: &Size{
				Constraints: []Constraint{
					{
						Type: CoreConstraint,
						Min:  1,
						Max:  1,
					},
					{
						Type: MemoryConstraint,
						Min:  1024,
						Max:  1024,
					},
					{
						Type: StorageConstraint,
						Min:  0,
						Max:  1024,
					},
				},
			},
			wantOverlap: true,
		},
		{
			name: "non overlapping size",
			this: &Size{
				Constraints: []Constraint{
					{
						Type: CoreConstraint,
						Min:  1,
						Max:  1,
					},
					{
						Type: MemoryConstraint,
						Min:  1024,
						Max:  1024,
					},
					{
						Type:       StorageConstraint,
						Identifier: "/dev/sd*",
						Min:        0,
						Max:        2048,
					},
				},
			},
			other: &Size{
				Constraints: []Constraint{
					{
						Type: CoreConstraint,
						Min:  1,
						Max:  1,
					},
					{
						Type: MemoryConstraint,
						Min:  1024,
						Max:  1024,
					},
					{
						Type:       StorageConstraint,
						Identifier: "/dev/nvme*",
						Min:        0,
						Max:        2024,
					},
				},
			},
			wantOverlap: false,
		},
		{
			name: "overlap, all the same",
			this: &Size{
				Constraints: []Constraint{
					{Type: MemoryConstraint, Identifier: "a", Min: 5, Max: 8},
					{Type: GPUConstraint, Identifier: "a", Min: 1, Max: 1},
					{Type: CoreConstraint, Min: 4, Max: 4},
				},
			},
			other: &Size{
				Constraints: []Constraint{
					{Type: CoreConstraint, Min: 4, Max: 4},
					{Type: GPUConstraint, Identifier: "a", Min: 1, Max: 1},
					{Type: MemoryConstraint, Identifier: "a", Min: 5, Max: 8},
				},
			},
			wantOverlap: true,
		},
		{
			name: "independent of order #1",
			this: &Size{
				Base: Base{
					ID: "g1-medium-x86",
				},
				Constraints: []Constraint{
					{
						Type: CoreConstraint,
						Min:  32,
						Max:  32,
					},
					{
						Type: MemoryConstraint,
						Min:  257698037760,
						Max:  300647710720,
					},
					{
						Type: StorageConstraint,
						Min:  1500000000000,
						Max:  2000000000000,
					},
					{
						Type:       GPUConstraint,
						Min:        1,
						Max:        1,
						Identifier: "AD102GL*",
					},
				},
			},
			other: &Size{
				Base: Base{
					ID: "c2-xlarge-x86",
				},
				Constraints: []Constraint{
					{
						Type: CoreConstraint,
						Min:  32,
						Max:  32,
					},
					{
						Type: MemoryConstraint,
						Min:  220000000000,
						Max:  280000000000,
					},
					{
						Type: StorageConstraint,
						Min:  500000000000,
						Max:  4000000000000,
					},
				},
			},
			wantOverlap: false,
		},
		{
			name: "independent of order #2",
			this: &Size{
				Base: Base{
					ID: "c2-xlarge-x86",
				},
				Constraints: []Constraint{
					{
						Type: CoreConstraint,
						Min:  32,
						Max:  32,
					},
					{
						Type: MemoryConstraint,
						Min:  220000000000,
						Max:  280000000000,
					},
					{
						Type: StorageConstraint,
						Min:  500000000000,
						Max:  4000000000000,
					},
				},
			},
			other: &Size{
				Base: Base{
					ID: "g1-medium-x86",
				},
				Constraints: []Constraint{
					{
						Type: CoreConstraint,
						Min:  32,
						Max:  32,
					},
					{
						Type: MemoryConstraint,
						Min:  257698037760,
						Max:  300647710720,
					},
					{
						Type: StorageConstraint,
						Min:  1500000000000,
						Max:  2000000000000,
					},
					{
						Type:       GPUConstraint,
						Min:        1,
						Max:        1,
						Identifier: "AD102GL*",
					},
				},
			},
			wantOverlap: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.this.overlaps(tt.other); !reflect.DeepEqual(got, tt.wantOverlap) {
				t.Errorf("Size.Overlaps() = %v, want %v", got, tt.wantOverlap)
			}
		})
	}
}
