package repository

import (
	"testing"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/stretchr/testify/require"
)

func Test_id(t *testing.T) {
	tests := []struct {
		name          string
		componentType apiv2.ComponentType
		identifier    string
		want          string
	}{
		{
			name:          "metal-core",
			componentType: apiv2.ComponentType_COMPONENT_TYPE_METAL_CORE,
			identifier:    "fel-wps1t01-r01leaf01",
			want:          "e20efc88-74b8-5325-99fb-daccb08c3004",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := id(tt.componentType.String(), tt.identifier)

			require.Equal(t, tt.want, got)
		})
	}
}
