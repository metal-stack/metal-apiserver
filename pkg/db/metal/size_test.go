package metal

import (
	"reflect"
	"testing"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/pkg/pointer"
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
			want: &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 1, Max: 1, Identifier: pointer.Pointer("Intel")},
		},
		{
			name: "memory constraint",
			c:    Constraint{Type: MemoryConstraint, Min: 1, Max: 1, Identifier: "Samsung"},
			want: &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1, Max: 1, Identifier: pointer.Pointer("Samsung")},
		},
		{
			name: "gpu constraint",
			c:    Constraint{Type: GPUConstraint, Min: 1, Max: 1, Identifier: "NVidia"},
			want: &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_GPU, Min: 1, Max: 1, Identifier: pointer.Pointer("NVidia")},
		},
		{
			name: "Storage constraint",
			c:    Constraint{Type: StorageConstraint, Min: 1, Max: 1, Identifier: "Kingston"},
			want: &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1, Max: 1, Identifier: pointer.Pointer("Kingston")},
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
			c:       &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES, Min: 1, Max: 1, Identifier: pointer.Pointer("Intel")},
			want:    &Constraint{Type: CoreConstraint, Min: 1, Max: 1, Identifier: "Intel"},
			wantErr: false,
		},

		{
			name: "memory constraint",
			c:    &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY, Min: 1, Max: 1, Identifier: pointer.Pointer("Samsung")},
			want: &Constraint{Type: MemoryConstraint, Min: 1, Max: 1, Identifier: "Samsung"},
		},
		{
			name: "gpu constraint",
			c:    &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_GPU, Min: 1, Max: 1, Identifier: pointer.Pointer("NVidia")},
			want: &Constraint{Type: GPUConstraint, Min: 1, Max: 1, Identifier: "NVidia"},
		},
		{
			name: "Storage constraint",
			c:    &apiv2.SizeConstraint{Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE, Min: 1, Max: 1, Identifier: pointer.Pointer("Kingston")},
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
