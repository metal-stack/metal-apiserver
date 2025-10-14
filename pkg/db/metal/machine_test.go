package metal

import "testing"

func TestMachineNetwork_ContainsIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		prefixes []string
		want     bool
	}{
		{
			name:     "contains",
			ip:       "1.2.3.4",
			prefixes: []string{"1.2.3.0/24"},
			want:     true,
		},
		{
			name:     "does not contain",
			ip:       "1.2.3.4",
			prefixes: []string{"1.2.2.0/24"},
			want:     false,
		},
		{
			name:     "does not panic on invalid ip address",
			ip:       "1.2.3.4.5",
			prefixes: []string{"1.2.3.0/24", "0.0.0.0/0"},
			want:     false,
		},
		{
			name:     "empty prefixes",
			ip:       "1.1.1.1",
			prefixes: []string{},
			want:     false,
		},
		{
			name:     "does not panic on invalid prefixes",
			ip:       "1.1.1.1",
			prefixes: []string{"1.2.3.4/64"},
			want:     false,
		},
		{
			name:     "positive match on 0.0.0.0/0",
			ip:       "1.1.1.1",
			prefixes: []string{"1.2.3.4/32", "0.0.0.0/0"},
			want:     true,
		},
		{
			name:     "positive match on 0.0.0.0/0",
			ip:       "1.1.1.1",
			prefixes: []string{"1.2.3.4/64", "0.0.0.0/0"},
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &MachineNetwork{
				Prefixes: tt.prefixes,
			}
			if got := n.ContainsIP(tt.ip); got != tt.want {
				t.Errorf("MachineNetwork.ContainsIP() = %v, want %v", got, tt.want)
			}
		})
	}
}
