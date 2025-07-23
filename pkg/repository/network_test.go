package repository

import (
	"reflect"
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

func Test_networkRepository_calculatePrefixDifferences(t *testing.T) {
	tests := []struct {
		name            string
		existingNetwork *metal.Network
		prefixes        []string
		wantToRemoved   metal.Prefixes
		wantToAdded     metal.Prefixes
		wantErr         bool
	}{
		{
			name:            "nothing to add or remove",
			existingNetwork: &metal.Network{Prefixes: metal.Prefixes{{IP: "10.0.0.0", Length: "8"}}},
			prefixes:        []string{"10.0.0.0/8"},
			wantToRemoved:   metal.Prefixes{},
			wantToAdded:     metal.Prefixes{},
			wantErr:         false,
		},
		{
			name:            "one to add none to remove",
			existingNetwork: &metal.Network{Prefixes: metal.Prefixes{{IP: "10.0.0.0", Length: "8"}}},
			prefixes:        []string{"10.0.0.0/8", "11.0.0.0/8"},
			wantToRemoved:   metal.Prefixes{},
			wantToAdded:     metal.Prefixes{{IP: "11.0.0.0", Length: "8"}},
			wantErr:         false,
		},
		{
			name:            "one to add one to remove",
			existingNetwork: &metal.Network{Prefixes: metal.Prefixes{{IP: "9.0.0.0", Length: "8"}, {IP: "10.0.0.0", Length: "8"}}},
			prefixes:        []string{"10.0.0.0/8", "11.0.0.0/8"},
			wantToRemoved:   metal.Prefixes{{IP: "9.0.0.0", Length: "8"}},
			wantToAdded:     metal.Prefixes{{IP: "11.0.0.0", Length: "8"}},
			wantErr:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &networkRepository{}

			gotToRemoved, gotToAdded, err := r.calculatePrefixDifferences(tt.existingNetwork, tt.prefixes)
			if (err != nil) != tt.wantErr {
				t.Errorf("networkRepository.calculatePrefixDifferences() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotToRemoved, tt.wantToRemoved) {
				t.Errorf("networkRepository.calculatePrefixDifferences() gotToRemoved = %v, want %v", gotToRemoved, tt.wantToRemoved)
			}
			if !reflect.DeepEqual(gotToAdded, tt.wantToAdded) {
				t.Errorf("networkRepository.calculatePrefixDifferences() gotToAdded = %v, want %v", gotToAdded, tt.wantToAdded)
			}
		})
	}
}
