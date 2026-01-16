package metal

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-lib/pkg/testcommon"
)

func TestToReplaceMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    apiv2.SwitchReplaceMode
		want    SwitchReplaceMode
		wantErr bool
	}{
		{
			name:    "unspecified",
			mode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_UNSPECIFIED,
			want:    "",
			wantErr: true,
		},
		{
			name:    "valid",
			mode:    apiv2.SwitchReplaceMode_SWITCH_REPLACE_MODE_OPERATIONAL,
			want:    SwitchReplaceModeOperational,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToReplaceMode(tt.mode)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToReplaceMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ToReplaceMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSwitch_TranslateNicMap(t *testing.T) {
	tests := []struct {
		name     string
		sw       *Switch
		targetOS SwitchOSVendor
		want     NicMap
		wantErr  bool
	}{
		{
			name: "both twins have the same os",
			sw: &Switch{
				Nics: []Nic{
					{Name: "swp1s0"},
					{Name: "swp1s1"},
					{Name: "swp1s2"},
					{Name: "swp1s3"},
				},
				OS: &SwitchOS{Vendor: SwitchOSVendorCumulus},
			},
			targetOS: SwitchOSVendorCumulus,
			want: map[string]*Nic{
				"swp1s0": {Name: "swp1s0"},
				"swp1s1": {Name: "swp1s1"},
				"swp1s2": {Name: "swp1s2"},
				"swp1s3": {Name: "swp1s3"},
			},
			wantErr: false,
		},
		{
			name: "cumulus to sonic",
			sw: &Switch{
				Nics: []Nic{
					{Name: "Ethernet1"},
					{Name: "Ethernet2"},
					{Name: "Ethernet3"},
					{Name: "Ethernet4"},
				},
				OS: &SwitchOS{Vendor: SwitchOSVendorSonic},
			},
			targetOS: SwitchOSVendorCumulus,
			want: map[string]*Nic{
				"swp1s1": {Name: "Ethernet1"},
				"swp1s2": {Name: "Ethernet2"},
				"swp1s3": {Name: "Ethernet3"},
				"swp2":   {Name: "Ethernet4"},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.sw.TranslateNicMap(tt.targetOS)
			if (err != nil) != tt.wantErr {
				t.Errorf("translateNicNames() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if cmp.Diff(got, tt.want) != "" {
				t.Errorf("translateNicNames() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSwitch_MapPortNames(t *testing.T) {
	tests := []struct {
		name     string
		sw       *Switch
		targetOS SwitchOSVendor
		want     map[string]string
		wantErr  bool
	}{
		{
			name: "same os",
			sw: &Switch{
				Nics: []Nic{
					{Name: "swp1s0"},
					{Name: "swp1s1"},
					{Name: "swp1s2"},
					{Name: "swp1s3"},
				},
				OS: &SwitchOS{Vendor: SwitchOSVendorCumulus},
			},
			targetOS: SwitchOSVendorCumulus,
			want: map[string]string{
				"swp1s0": "swp1s0",
				"swp1s1": "swp1s1",
				"swp1s2": "swp1s2",
				"swp1s3": "swp1s3",
			},
			wantErr: false,
		},
		{
			name: "cumulus to sonic",
			sw: &Switch{
				Nics: []Nic{
					{Name: "swp1s0"},
					{Name: "swp2s0"},
					{Name: "swp2s1"},
					{Name: "swp2s2"},
				},
				OS: &SwitchOS{Vendor: SwitchOSVendorCumulus},
			},
			targetOS: SwitchOSVendorSonic,
			want: map[string]string{
				"swp1s0": "Ethernet0",
				"swp2s0": "Ethernet4",
				"swp2s1": "Ethernet5",
				"swp2s2": "Ethernet6",
			},
			wantErr: false,
		},
		{
			name: "sonic to cumulus",
			sw: &Switch{
				Nics: []Nic{
					{Name: "Ethernet0"},
					{Name: "Ethernet4"},
					{Name: "Ethernet8"},
					{Name: "Ethernet9"},
				},
				OS: &SwitchOS{Vendor: SwitchOSVendorSonic},
			},
			targetOS: SwitchOSVendorCumulus,
			want: map[string]string{
				"Ethernet0": "swp1",
				"Ethernet4": "swp2",
				"Ethernet8": "swp3s0",
				"Ethernet9": "swp3s1",
			},
			wantErr: false,
		},
		{
			name: "sonic names in cumulus switch",
			sw: &Switch{
				Nics: []Nic{
					{Name: "Ethernet0"},
					{Name: "Ethernet4"},
					{Name: "Ethernet8"},
					{Name: "Ethernet9"},
				},
				OS: &SwitchOS{Vendor: SwitchOSVendorCumulus},
			},
			targetOS: SwitchOSVendorSonic,
			want:     nil,
			wantErr:  true,
		},
		{
			name: "cumulus names in sonic switch",
			sw: &Switch{
				Nics: []Nic{
					{Name: "swp1s0"},
					{Name: "swp1s1"},
					{Name: "swp1s2"},
					{Name: "swp1s3"},
				},
				OS: &SwitchOS{Vendor: SwitchOSVendorSonic},
			},
			targetOS: SwitchOSVendorCumulus,
			want:     nil,
			wantErr:  true,
		},
		{
			name: "invalid name",
			sw: &Switch{
				Nics: []Nic{
					{Name: "swp1s"},
				},
				OS: &SwitchOS{Vendor: SwitchOSVendorSonic},
			},
			targetOS: SwitchOSVendorCumulus,
			want:     nil,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.sw.MapPortNames(tt.targetOS)
			if (err != nil) != tt.wantErr {
				t.Errorf("Switch.MapPortNames() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("%v", diff)
			}
		})
	}
}

func Test_mapPortName(t *testing.T) {
	tests := []struct {
		name     string
		port     string
		sourceOS SwitchOSVendor
		targetOS SwitchOSVendor
		allLines []int
		want     string
		wantErr  error
	}{
		{
			name:     "invalid target os",
			port:     "Ethernet0",
			sourceOS: SwitchOSVendorSonic,
			targetOS: "cumulus",
			allLines: []int{0, 1},
			want:     "",
			wantErr:  fmt.Errorf("unknown target switch os cumulus"),
		},
		{
			name:     "sonic to cumulus",
			port:     "Ethernet11",
			sourceOS: SwitchOSVendorSonic,
			targetOS: SwitchOSVendorCumulus,
			allLines: []int{11},
			want:     "swp3s3",
			wantErr:  nil,
		},
		{
			name:     "cumulus to sonic",
			port:     "swp4s0",
			sourceOS: SwitchOSVendorCumulus,
			targetOS: SwitchOSVendorSonic,
			allLines: []int{0, 4, 12, 13},
			want:     "Ethernet12",
			wantErr:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapPortName(tt.port, tt.sourceOS, tt.targetOS, tt.allLines)
			if diff := cmp.Diff(err, tt.wantErr, testcommon.ErrorStringComparer()); diff != "" {
				t.Errorf("MapPortName() error diff: %s", diff)
				return
			}
			if got != tt.want {
				t.Errorf("MapPortName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getLinesFromPortNames(t *testing.T) {
	tests := []struct {
		name    string
		ports   []string
		os      SwitchOSVendor
		want    []int
		wantErr bool
	}{
		{
			name:    "invalid switch os",
			ports:   []string{"swp1", "swp1s2"},
			os:      "cumulus",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "mismatch between port names and os cumulus",
			ports:   []string{"Ethernet0", "Ethernet1"},
			os:      SwitchOSVendorCumulus,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "mismatch between port names and os sonic",
			ports:   []string{"swp1s0", "swp1s1"},
			os:      SwitchOSVendorSonic,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "sonic conversion successful",
			ports:   []string{"Ethernet0", "Ethernet2"},
			os:      SwitchOSVendorSonic,
			want:    []int{0, 2},
			wantErr: false,
		},
		{
			name:    "cumulus conversion successful",
			ports:   []string{"swp1", "swp2s3"},
			os:      SwitchOSVendorCumulus,
			want:    []int{0, 7},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getLinesFromPortNames(tt.ports, tt.os)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetLinesFromPortNames() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetLinesFromPortNames() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_sonicPortNameToLine(t *testing.T) {
	_, parseIntError := strconv.Atoi("_1")

	tests := []struct {
		name    string
		port    string
		want    int
		wantErr error
	}{
		{
			name:    "invalid token",
			port:    "Ethernet-0",
			want:    0,
			wantErr: fmt.Errorf("invalid token '-' in port name Ethernet-0"),
		},
		{
			name:    "missing prefix 'Ethernet'",
			port:    "swp1s0",
			want:    0,
			wantErr: fmt.Errorf("invalid port name swp1s0, expected to find prefix 'Ethernet'"),
		},
		{
			name:    "invalid prefix before 'Ethernet'",
			port:    "port_Ethernet0",
			want:    0,
			wantErr: fmt.Errorf("invalid port name port_Ethernet0, port name is expected to start with 'Ethernet'"),
		},
		{
			name:    "cannot convert line number",
			port:    "Ethernet_1",
			want:    0,
			wantErr: fmt.Errorf("unable to convert port name to line number: %w", parseIntError),
		},
		{
			name:    "conversion successful",
			port:    "Ethernet25",
			want:    25,
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sonicPortNameToLine(tt.port)
			if diff := cmp.Diff(err, tt.wantErr, testcommon.ErrorStringComparer()); diff != "" {
				t.Errorf("sonicPortNameToLine() error diff: %s", diff)
				return
			}
			if got != tt.want {
				t.Errorf("sonicPortNameToLine() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cumulusPortNameToLine(t *testing.T) {
	_, parseIntError1 := strconv.Atoi("1t0")
	_, parseIntError2 := strconv.Atoi("_0")

	tests := []struct {
		name    string
		port    string
		want    int
		wantErr error
	}{
		{
			name:    "invalid token",
			port:    "swp-0s1",
			want:    0,
			wantErr: fmt.Errorf("invalid token '-' in port name swp-0s1"),
		},
		{
			name:    "missing prefix 'swp'",
			port:    "Ethernet0",
			want:    0,
			wantErr: fmt.Errorf("invalid port name Ethernet0, expected to find prefix 'swp'"),
		},
		{
			name:    "invalid prefix before 'swp'",
			port:    "port_swp1s0",
			want:    0,
			wantErr: fmt.Errorf("invalid port name port_swp1s0, port name is expected to start with 'swp'"),
		},
		{
			name:    "wrong delimiter",
			port:    "swp1t0",
			want:    0,
			wantErr: fmt.Errorf("unable to convert port name to line number: %w", parseIntError1),
		},
		{
			name:    "cannot convert first number",
			port:    "swp_0s0",
			want:    0,
			wantErr: fmt.Errorf("unable to convert port name to line number: %w", parseIntError2),
		},
		{
			name:    "cannot convert second number",
			port:    "swp1s_0",
			want:    0,
			wantErr: fmt.Errorf("unable to convert port name to line number: %w", parseIntError2),
		},
		{
			name:    "cannot convert swp0 because that would result in a negative line number",
			port:    "swp0",
			want:    0,
			wantErr: fmt.Errorf("invalid port name swp0 would map to negative number"),
		},
		{
			name:    "cannot convert swp0s1 because that would result in a negative line number",
			port:    "swp0s1",
			want:    0,
			wantErr: fmt.Errorf("invalid port name swp0s1 would map to negative number"),
		},
		{
			name:    "convert line without breakout",
			port:    "swp4",
			want:    12,
			wantErr: nil,
		},
		{
			name:    "convert line with breakout",
			port:    "swp3s3",
			want:    11,
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cumulusPortNameToLine(tt.port)
			if diff := cmp.Diff(err, tt.wantErr, testcommon.ErrorStringComparer()); diff != "" {
				t.Errorf("cumulusPortNameToLine() error diff: %s", diff)
				return
			}
			if got != tt.want {
				t.Errorf("cumulusPortNameToLine() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cumulusPortByLineNumber(t *testing.T) {
	tests := []struct {
		name     string
		line     int
		allLines []int
		want     string
	}{
		{
			name:     "only one line",
			line:     4,
			allLines: []int{4},
			want:     "swp2",
		},
		{
			name:     "line number 0 without breakout",
			line:     0,
			allLines: []int{0, 4},
			want:     "swp1",
		},
		{
			name:     "higher line number without breakout",
			line:     4,
			allLines: []int{0, 1, 2, 3, 4, 8},
			want:     "swp2",
		},
		{
			name:     "line number divisible by 4 with breakout",
			line:     4,
			allLines: []int{4, 5, 6, 7},
			want:     "swp2s0",
		},
		{
			name:     "line number not divisible by 4",
			line:     13,
			allLines: []int{13},
			want:     "swp4s1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cumulusPortByLineNumber(tt.line, tt.allLines); got != tt.want {
				t.Errorf("cumulusPortByLineNumber() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSwitch_getPhysicalMachineConnection(t *testing.T) {
	tests := []struct {
		name        string
		s           *Switch
		machineID   string
		machineNics Nics
		want        Connections
	}{
		{
			name: "machine is connected",
			s: &Switch{
				Base: Base{
					ID: "leaf01",
				},
				Nics: Nics{
					{
						MacAddress: "aa:aa:aa:aa:aa:aa",
						Name:       "Ethernet12",
						Identifier: "Eth4",
						Hostname:   "leaf01",
					},
				},
			},
			machineID: "m1",
			machineNics: Nics{
				{
					Neighbors: Nics{
						{
							MacAddress: "aa:aa:aa:aa:aa:aa",
							Identifier: "Eth4",
							Hostname:   "leaf01",
						},
					},
				},
			},
			want: Connections{
				{
					Nic: Nic{
						MacAddress: "aa:aa:aa:aa:aa:aa",
						Name:       "Ethernet12",
						Identifier: "Eth4",
						Hostname:   "leaf01",
					},
					MachineID: "m1",
				},
			},
		},
		{
			name: "machine is not connected",
			s: &Switch{
				Base: Base{
					ID: "leaf02",
				},
				Nics: Nics{
					{
						MacAddress: "aa:aa:aa:aa:aa:aa",
						Name:       "Ethernet12",
						Identifier: "Eth4",
						Hostname:   "leaf02",
					},
				},
			},
			machineID: "m1",
			machineNics: Nics{
				{
					Neighbors: Nics{
						{
							MacAddress: "bb:bb:bb:bb:bb:bb",
							Identifier: "Eth4",
							Hostname:   "leaf01",
						},
					},
				},
			},

			want: Connections{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.s.getPhysicalMachineConnections(tt.machineID, tt.machineNics)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Switch.getPhysicalMachineConnection() diff = %v", diff)
			}
		})
	}
}

func TestSwitch_ConnectMachine(t *testing.T) {
	tests := []struct {
		name            string
		s               *Switch
		machineID       string
		machineNics     Nics
		want            int
		wantConnections ConnectionMap
		wantErr         bool
	}{
		{
			name: "switch and machine are not connected",
			s: &Switch{
				Base: Base{
					ID: "sw1",
				},
				Nics: Nics{
					{
						MacAddress: "aa:aa:aa:aa:aa:aa",
						Name:       "Ethernet12",
						Identifier: "Eth4",
						Hostname:   "sw1",
					},
				},
				MachineConnections: ConnectionMap{
					"m2": {
						{
							Nic: Nic{
								MacAddress: "aa:aa:aa:aa:aa:aa",
								Name:       "Ethernet12",
								Identifier: "Eth4",
								Hostname:   "sw1",
							},
							MachineID: "m2",
						},
					},
				},
			},
			machineID: "m1",
			machineNics: Nics{
				{
					Neighbors: Nics{
						{
							MacAddress: "ee:ee:ee:ee:ee:ee",
							Name:       "Ethernet12",
							Identifier: "Eth4",
							Hostname:   "sw2",
						},
					},
				},
			},
			want: 0,
			wantConnections: ConnectionMap{
				"m2": {
					{
						Nic: Nic{
							MacAddress: "aa:aa:aa:aa:aa:aa",
							Name:       "Ethernet12",
							Identifier: "Eth4",
							Hostname:   "sw1",
						},
						MachineID: "m2",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "error when machine connection for the switch exists in the database but not physically",
			s: &Switch{
				Base: Base{
					ID: "sw1",
				},
				Nics: Nics{
					{
						MacAddress: "aa:aa:aa:aa:aa:aa",
						Name:       "Ethernet12",
						Identifier: "Eth4",
						Hostname:   "sw1",
					},
				},
				MachineConnections: ConnectionMap{
					"m1": {
						{
							Nic: Nic{
								MacAddress: "aa:aa:aa:aa:aa:aa",
								Name:       "Ethernet12",
								Identifier: "Eth4",
							},
							MachineID: "m1",
						},
					},
				},
			},
			machineID: "m1",
			machineNics: Nics{
				{
					Neighbors: Nics{
						{
							Hostname: "sw2",
						},
					},
				},
			},
			want: 0,
			wantConnections: ConnectionMap{
				"m1": {
					{
						Nic: Nic{
							MacAddress: "aa:aa:aa:aa:aa:aa",
							Name:       "Ethernet12",
							Identifier: "Eth4",
						},
						MachineID: "m1",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "new connection replaces old ones for the same switch",
			s: &Switch{
				Base: Base{
					ID: "sw1",
				},
				Nics: Nics{
					{
						MacAddress: "bb:bb:bb:bb:bb:bb",
						Name:       "Ethernet16",
						Identifier: "Eth5",
					},
				},
				MachineConnections: ConnectionMap{
					"m1": {
						{
							Nic: Nic{
								MacAddress: "aa:aa:aa:aa:aa:aa",
								Name:       "Ethernet12",
								Identifier: "Eth4",
							},
							MachineID: "",
						},
					},
					"m2": {
						{
							Nic: Nic{
								MacAddress: "cc:cc:cc:cc:cc:cc",
								Name:       "Ethernet20",
								Identifier: "Eth6",
							},
							MachineID: "m2",
						},
					},
				},
			},
			machineID: "m1",
			machineNics: Nics{
				{
					Neighbors: Nics{
						{
							MacAddress: "bb:bb:bb:bb:bb:bb",
							Name:       "Ethernet16",
							Identifier: "Eth5",
							Hostname:   "sw1",
						},
					},
				},
			},
			want: 1,
			wantConnections: ConnectionMap{
				"m1": {
					{
						Nic: Nic{
							MacAddress: "bb:bb:bb:bb:bb:bb",
							Name:       "Ethernet16",
							Identifier: "Eth5",
						},
						MachineID: "m1",
					},
				},
				"m2": {
					{
						Nic: Nic{
							MacAddress: "cc:cc:cc:cc:cc:cc",
							Name:       "Ethernet20",
							Identifier: "Eth6",
						},
						MachineID: "m2",
					},
				},
			},
			wantErr: false,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.s.ConnectMachine(tt.machineID, tt.machineNics)
			if (err != nil) != tt.wantErr {
				t.Errorf("Switch.ConnectMachine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got != tt.want {
				t.Errorf("Switch.ConnectMachine() = %v, want %v", got, tt.want)
				return
			}

			if diff := cmp.Diff(tt.wantConnections, tt.s.MachineConnections); diff != "" {
				t.Errorf("Switch.ConnectMachine() diff = %v", diff)
			}
		})
	}
}

func TestConnectionMap_ByNicName(t *testing.T) {
	tests := []struct {
		name           string
		c              ConnectionMap
		want           map[string]Connection
		wantErr        bool
		wantErrmessage string
	}{
		{
			name: "one machine connected to one switch",
			c: ConnectionMap{
				"m1": Connections{
					Connection{MachineID: "m1", Nic: Nic{MacAddress: "11:11", Name: "swp1"}},
				},
			},
			want: map[string]Connection{
				"swp1": {MachineID: "m1", Nic: Nic{MacAddress: "11:11", Name: "swp1"}},
			},
			wantErr: false,
		},
		{
			name: "two machines connected to one switch",
			c: ConnectionMap{
				"m1": Connections{
					Connection{MachineID: "m1", Nic: Nic{MacAddress: "11:11", Name: "swp1"}},
				},
				"m2": Connections{
					Connection{MachineID: "m2", Nic: Nic{MacAddress: "21:11", Name: "swp2"}},
				},
			},
			want: map[string]Connection{
				"swp1": {MachineID: "m1", Nic: Nic{MacAddress: "11:11", Name: "swp1"}},
				"swp2": {MachineID: "m2", Nic: Nic{MacAddress: "21:11", Name: "swp2"}},
			},
			wantErr: false,
		},
		{
			name: "two machines connected to one switch at the same port",
			c: ConnectionMap{
				"m1": Connections{
					Connection{MachineID: "m1", Nic: Nic{MacAddress: "11:11", Name: "swp1"}},
				},
				"m2": Connections{
					Connection{MachineID: "m2", Nic: Nic{MacAddress: "21:11", Name: "swp1"}},
				},
			},
			want:           nil,
			wantErr:        true,
			wantErrmessage: "switch port swp1 is connected to more than one machine",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.c.ByNicName()
			if (err != nil) != tt.wantErr {
				t.Errorf("ConnectionMap.ByNicName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantErrmessage != err.Error() {
				t.Errorf("ConnectionMap.ByNicName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ConnectionMap.ByNicName() diff: %s", diff)
			}
		})
	}
}

func TestFromBGPState(t *testing.T) {
	tests := []struct {
		name    string
		state   BGPState
		want    apiv2.BGPState
		wantErr bool
	}{
		{
			name:    "invalid bgp state",
			state:   "invalid",
			want:    0,
			wantErr: true,
		},
		{
			name:    "bgp state established",
			state:   "Established", // this is a string and not the enum value on purpose
			want:    apiv2.BGPState_BGP_STATE_ESTABLISHED,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromBGPState(tt.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromBGPState() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromBGPState() = %v, want %v", got, tt.want)
			}
		})
	}
}
