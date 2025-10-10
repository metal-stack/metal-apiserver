package repository

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-lib/pkg/testcommon"
)

func Test_checkDuplicateNics(t *testing.T) {
	tests := []struct {
		name    string
		nics    metal.Nics
		wantErr error
	}{
		{
			name: "no duplicates found",
			nics: metal.Nics{
				{
					Name:       "Ethernet0",
					Identifier: "Eth1/1",
				},
				{
					Name:       "Ethernet1",
					Identifier: "Eth1/2",
				},
			},
			wantErr: nil,
		},
		{
			name: "duplicate identifiers",
			nics: metal.Nics{
				{
					Name:       "Ethernet0",
					Identifier: "Eth1/1",
				},
				{
					Name:       "Ethernet1",
					Identifier: "Eth1/1",
				},
			},
			wantErr: errors.Join(fmt.Errorf("switch nics contain duplicate identifiers:[Eth1/1]")),
		},
		{
			name: "duplicate names",
			nics: metal.Nics{
				{
					Name:       "Ethernet0",
					Identifier: "Eth1/1",
				},
				{
					Name:       "Ethernet0",
					Identifier: "Eth1/2",
				},
			},
			wantErr: errors.Join(fmt.Errorf("switch nics contain duplicate names:[Ethernet0]")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkDuplicateNics(tt.nics)
			if diff := cmp.Diff(tt.wantErr, err, testcommon.ErrorStringComparer()); diff != "" {
				t.Errorf("checkDuplicateNics() error diff = %s", diff)
			}
		})
	}
}

func Test_validateConnectedNics(t *testing.T) {
	tests := []struct {
		name        string
		old         metal.Nics
		new         metal.Nics
		connections metal.ConnectionMap
		wantErr     error
	}{
		{
			name: "cannot remove connected nics",
			old: metal.Nics{
				{
					Identifier: "Eth1/1",
				},
				{
					Identifier: "Eth1/2",
				},
				{
					Identifier: "Eth1/3",
				},
				{
					Identifier: "Eth1/4",
				},
			},
			new: metal.Nics{
				{
					Identifier: "Eth1/1",
				},
				{
					Identifier: "Eth1/2",
				},
				{
					Identifier: "Eth1/3",
				},
			},
			connections: metal.ConnectionMap{
				"machine-a": metal.Connections{
					{
						Nic: metal.Nic{
							Identifier: "Eth1/1",
						},
					},
				},
				"machine-b": metal.Connections{
					{
						Nic: metal.Nic{
							Identifier: "Eth1/2",
						},
					},
				},
				"machine-c": metal.Connections{
					{
						Nic: metal.Nic{
							Identifier: "Eth1/3",
						},
					},
				},
				"machine-d": metal.Connections{
					{
						Nic: metal.Nic{
							Identifier: "Eth1/4",
						},
					},
				},
			},
			wantErr: errors.Join(fmt.Errorf("cannot remove nics [Eth1/4] because they are connected to machines")),
		},
		{
			name: "cannot rename connected nics",
			old: metal.Nics{
				{
					Identifier: "Eth1/1",
					Name:       "Ethernet0",
				},
				{
					Identifier: "Eth1/2",
					Name:       "Ethernet1",
				},
				{
					Identifier: "Eth1/3",
					Name:       "Ethernet2",
				},
				{
					Identifier: "Eth1/4",
					Name:       "Ethernet3",
				},
			},
			new: metal.Nics{
				{
					Identifier: "Eth1/1",
					Name:       "Ethernet0",
				},
				{
					Identifier: "Eth1/2",
					Name:       "Ethernet1",
				},
				{
					Identifier: "Eth1/3",
					Name:       "Ethernet3",
				},
				{
					Identifier: "Eth1/4",
					Name:       "Ethernet2",
				},
			},
			connections: metal.ConnectionMap{
				"machine-a": metal.Connections{
					{
						Nic: metal.Nic{
							Identifier: "Eth1/1",
						},
					},
				},
				"machine-b": metal.Connections{
					{
						Nic: metal.Nic{
							Identifier: "Eth1/2",
						},
					},
				},
				"machine-c": metal.Connections{
					{
						Nic: metal.Nic{
							Identifier: "Eth1/3",
						},
					},
				},
				"machine-d": metal.Connections{
					{
						Nic: metal.Nic{
							Identifier: "Eth1/4",
						},
					},
				},
			},
			wantErr: errors.Join(fmt.Errorf("cannot rename nics [Eth1/3 Eth1/4] because they are connected to machines")),
		},
		{
			name: "all valid",
			old: metal.Nics{
				{
					Identifier: "Eth1/1",
					Name:       "Ethernet0",
				},
				{
					Identifier: "Eth1/2",
					Name:       "Ethernet1",
				},
				{
					Identifier: "Eth1/3",
					Name:       "Ethernet2",
				},
				{
					Identifier: "Eth1/4",
					Name:       "Ethernet3",
				},
			},
			new: metal.Nics{
				{
					Identifier: "Eth1/1",
					Name:       "Ethernet0",
				},
				{
					Identifier: "Eth1/2",
					Name:       "Ethernet1",
				},
				{
					Identifier: "Eth1/3",
					Name:       "Ethernet3",
				},
			},
			connections: metal.ConnectionMap{
				"machine-a": metal.Connections{
					{
						Nic: metal.Nic{
							Identifier: "Eth1/1",
						},
					},
				},
				"machine-b": metal.Connections{
					{
						Nic: metal.Nic{
							Identifier: "Eth1/2",
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConnectedNics(tt.old, tt.new, tt.connections)
			if diff := cmp.Diff(tt.wantErr, err, testcommon.ErrorStringComparer()); diff != "" {
				t.Errorf("validateConnectedNics() error diff = %s", diff)
			}
		})
	}
}

func Test_nicIsConnected(t *testing.T) {
	tests := []struct {
		name        string
		identifier  string
		connections metal.ConnectionMap
		want        bool
	}{
		{
			name:       "not connected",
			identifier: "Eth1/1",
			connections: metal.ConnectionMap{
				"machine-a": metal.Connections{
					{
						Nic: metal.Nic{
							Identifier: "Eth1/2",
						},
					},
				},
			},
			want: false,
		},
		{
			name:       "connected",
			identifier: "Eth1/1",
			connections: metal.ConnectionMap{
				"machine-a": metal.Connections{
					{
						Nic: metal.Nic{
							Identifier: "Eth1/2",
						},
					},
				},
				"machine-b": metal.Connections{
					{
						Nic: metal.Nic{
							Identifier: "Eth2/1",
						},
					},
					{
						Nic: metal.Nic{
							Identifier: "Eth1/1",
						},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nicIsConnected(tt.identifier, tt.connections)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("nicIsConnected() diff = %s", diff)
			}
		})
	}
}
