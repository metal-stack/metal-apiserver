package metal

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNics_MapByName(t *testing.T) {
	tests := []struct {
		name string
		nics Nics
		want NicMap
	}{
		{
			name: "empty nics returns empty map",
			nics: Nics{},
			want: NicMap{},
		},
		{
			name: "nics with empty name are omitted",
			nics: Nics{
				{Name: ""},
			},
			want: NicMap{},
		},
		{
			name: "map all nics",
			nics: Nics{
				{Name: "Ethernet0"},
				{Name: "Ethernet1"},
				{Name: "Ethernet2"},
			},
			want: NicMap{
				"Ethernet0": {Name: "Ethernet0"},
				"Ethernet1": {Name: "Ethernet1"},
				"Ethernet2": {Name: "Ethernet2"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.nics.MapByName()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Nics.MapByName() diff = %s", diff)
			}
		})
	}
}

func TestNics_MapByIdentifier(t *testing.T) {
	tests := []struct {
		name string
		nics Nics
		want NicMap
	}{
		{
			name: "empty nics returns empty map",
			nics: Nics{},
			want: NicMap{},
		},
		{
			name: "nics with empty identifier are omitted",
			nics: Nics{
				{Identifier: ""},
			},
			want: NicMap{},
		},
		{
			name: "map all nics",
			nics: Nics{
				{Identifier: "Eth0"},
				{Identifier: "Eth1"},
				{Identifier: "Eth2"},
			},
			want: NicMap{
				"Eth0": {Identifier: "Eth0"},
				"Eth1": {Identifier: "Eth1"},
				"Eth2": {Identifier: "Eth2"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.nics.MapByIdentifier()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Nics.MapByIdentifier() diff = %s", diff)
			}
		})
	}
}

func TestNics_FilterByHostname(t *testing.T) {
	tests := []struct {
		name     string
		nics     Nics
		hostname string
		want     Nics
	}{
		{
			name: "empty hostname",
			nics: Nics{
				{Name: "Ethernet0"},
				{Name: "Ethernet1"},
				{Name: "Ethernet2"},
			},
			hostname: "",
			want: Nics{
				{Name: "Ethernet0"},
				{Name: "Ethernet1"},
				{Name: "Ethernet2"},
			},
		},
		{
			name: "filter by hostname",
			nics: Nics{
				{Name: "Ethernet0", Hostname: "leaf01"},
				{Name: "Ethernet1", Hostname: "leaf02"},
				{Name: "Ethernet2", Hostname: "leaf01"},
			},
			hostname: "leaf01",
			want: Nics{
				{Name: "Ethernet0", Hostname: "leaf01"},
				{Name: "Ethernet2", Hostname: "leaf01"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.nics.FilterByHostname(tt.hostname)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Nics.FilterByHostname() diff = %s", diff)
			}
		})
	}
}
