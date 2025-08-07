package repository

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

func Test_sizeRepository_validateSizeConstraints(t *testing.T) {

	tests := []struct {
		name        string
		constraints []*apiv2.SizeConstraint
		wantErr     error
	}{
		{
			name: "cpu min and max wrong",
			constraints: []*apiv2.SizeConstraint{
				{
					Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES,
					Min:  8,
					Max:  2,
				},
			},
			wantErr: errors.New("constraint at index 0 is invalid: max is smaller than min"),
		},
		{
			name: "memory min and max wrong",
			constraints: []*apiv2.SizeConstraint{
				{
					Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY,
					Min:  8,
					Max:  2,
				},
			},
			wantErr: errors.New("constraint at index 0 is invalid: max is smaller than min"),
		},
		{
			name: "storage is working",
			constraints: []*apiv2.SizeConstraint{
				{
					Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE,
					Min:  2,
					Max:  8,
				},
			},
			wantErr: nil,
		},
		{
			name: "two gpu constraints are allowed",
			constraints: []*apiv2.SizeConstraint{
				{
					Type:       apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_GPU,
					Min:        1,
					Max:        1,
					Identifier: pointer.Pointer("A100*"),
				},
				{
					Type:       apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_GPU,
					Min:        2,
					Max:        2,
					Identifier: pointer.Pointer("H100*"),
				},
			},
			wantErr: nil,
		},
		{
			name: "two cpu constraints are not allowed",
			constraints: []*apiv2.SizeConstraint{
				{
					Type:       apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES,
					Min:        1,
					Max:        1,
					Identifier: pointer.Pointer("Intel Xeon Silver"),
				},
				{
					Type:       apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_CORES,
					Min:        2,
					Max:        2,
					Identifier: pointer.Pointer("Intel Xeon Gold"),
				},
			},
			wantErr: errors.New("constraint at index 1 is invalid: type duplicates are not allowed for type \"SIZE_CONSTRAINT_TYPE_CORES\""),
		},
		{
			name: "gpu size without identifier",
			constraints: []*apiv2.SizeConstraint{
				{
					Type: apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_GPU,
					Min:  2,
					Max:  8,
				},
			},
			wantErr: errors.New("constraint at index 0 is invalid: for gpu constraints an identifier is required"),
		},
		{
			name: "storage with invalid identifier",
			constraints: []*apiv2.SizeConstraint{
				{
					Type:       apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_STORAGE,
					Identifier: pointer.Pointer("]["),
					Min:        2,
					Max:        8,
				},
			},
			wantErr: errors.New("constraint at index 0 is invalid: identifier is malformed: syntax error in pattern"),
		},
		{
			name: "memory with identifier",
			constraints: []*apiv2.SizeConstraint{
				{
					Type:       apiv2.SizeConstraintType_SIZE_CONSTRAINT_TYPE_MEMORY,
					Identifier: pointer.Pointer("Kingston"),
					Min:        2,
					Max:        8,
				},
			},
			wantErr: errors.New("constraint at index 0 is invalid: for memory constraints an identifier is not allowed"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &sizeRepository{}
			err := r.validateSizeConstraints(tt.constraints)
			if err == nil && tt.wantErr == nil {
				return
			}
			if tt.wantErr == nil && err != nil {
				t.Errorf("wantErr is nil but err is not nil %v", err)
			}
			if diff := cmp.Diff(err.Error(), tt.wantErr.Error()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}
