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

func TestEgressRule_Validate(t *testing.T) {
	tests := []struct {
		name       string
		Protocol   Protocol
		Ports      []int
		To         []string
		Comment    string
		wantErr    bool
		wantErrmsg string
	}{
		{
			name:     "valid egress rule",
			Protocol: ProtocolTCP,
			Ports:    []int{1, 2, 3},
			To:       []string{"1.2.3.0/24", "2.3.4.5/32"},
			Comment:  "allow apt update",
		},
		{
			name:       "wrong protocol",
			Protocol:   Protocol("sctp"),
			Ports:      []int{1, 2, 3},
			To:         []string{"1.2.3.0/24", "2.3.4.5/32"},
			Comment:    "allow apt update",
			wantErr:    true,
			wantErrmsg: "egress rule has invalid protocol: sctp",
		},
		{
			name:       "wrong port",
			Protocol:   ProtocolTCP,
			Ports:      []int{1, 2, 3, -1},
			To:         []string{"1.2.3.0/24", "2.3.4.5/32"},
			Comment:    "allow apt update",
			wantErr:    true,
			wantErrmsg: "egress rule with error:port -1 is out of range",
		},
		{
			name:       "wrong cidr",
			Protocol:   ProtocolTCP,
			Ports:      []int{1, 2, 3},
			To:         []string{"1.2.3.0/24", "2.3.4.5/33"},
			Comment:    "allow apt update",
			wantErr:    true,
			wantErrmsg: "egress rule with error:invalid cidr: netip.ParsePrefix(\"2.3.4.5/33\"): prefix length out of range",
		},
		{
			name:       "wrong comment",
			Protocol:   ProtocolTCP,
			Ports:      []int{1, 2, 3},
			To:         []string{"1.2.3.0/24", "2.3.4.5/32"},
			Comment:    "allow apt update\n",
			wantErr:    true,
			wantErrmsg: `egress rule with error:illegal character '\n' in comment found, only: "abcdefghijklmnopqrstuvwxyz_- " allowed`,
		},
		{
			name:       "too long comment",
			Protocol:   ProtocolTCP,
			Ports:      []int{1, 2, 3},
			To:         []string{"1.2.3.0/24", "2.3.4.5/32"},
			Comment:    "much too long comment aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantErr:    true,
			wantErrmsg: "egress rule with error:comments can not exceed 100 characters",
		},
		{
			name:       "mixed address family in cidrs",
			Protocol:   ProtocolTCP,
			Ports:      []int{1, 2, 3},
			To:         []string{"1.2.3.0/24", "2.3.4.5/32", "2001:db8::/32"},
			Comment:    "mixed address family",
			wantErr:    true,
			wantErrmsg: "egress rule with error:mixed address family in one rule is not supported:[1.2.3.0/24 2.3.4.5/32 2001:db8::/32]",
		},
		{
			name:       "malformed cidr",
			Protocol:   ProtocolTCP,
			Ports:      []int{1, 2, 3},
			To:         []string{"2001:db8::1"},
			Comment:    "malformed cidr",
			wantErr:    true,
			wantErrmsg: "egress rule with error:invalid cidr: netip.ParsePrefix(\"2001:db8::1\"): no '/'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := EgressRule{
				Protocol: tt.Protocol,
				Ports:    tt.Ports,
				To:       tt.To,
				Comment:  tt.Comment,
			}
			if err := r.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("EgressRule.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := r.Validate(); err != nil {
				if tt.wantErrmsg != err.Error() {
					t.Errorf("EgressRule.Validate() error = %v, wantErrmsg %v", err.Error(), tt.wantErrmsg)
				}
			}
		})
	}
}
func TestIngressRule_Validate(t *testing.T) {
	tests := []struct {
		name       string
		Protocol   Protocol
		Ports      []int
		To         []string
		From       []string
		Comment    string
		wantErr    bool
		wantErrmsg string
	}{
		{
			name:     "valid ingress rule",
			Protocol: ProtocolTCP,
			Ports:    []int{1, 2, 3},
			From:     []string{"1.2.3.0/24", "2.3.4.5/32"},
			Comment:  "allow apt update",
		},
		{
			name:     "valid ingress rule",
			Protocol: ProtocolTCP,
			Ports:    []int{1, 2, 3},
			From:     []string{"1.2.3.0/24", "2.3.4.5/32"},
			To:       []string{"100.2.3.0/24", "200.3.4.5/32"},
			Comment:  "allow apt update",
		},
		{
			name:       "invalid ingress rule, mixed address families in to and from",
			Protocol:   ProtocolTCP,
			Ports:      []int{1, 2, 3},
			From:       []string{"1.2.3.0/24", "2.3.4.5/32"},
			To:         []string{"100.2.3.0/24", "2001:db8::/32"},
			Comment:    "allow apt update",
			wantErr:    true,
			wantErrmsg: "ingress rule with error:mixed address family in one rule is not supported:[100.2.3.0/24 2001:db8::/32]",
		},
		{
			name:       "invalid ingress rule, mixed address families in to and from",
			Protocol:   ProtocolTCP,
			Ports:      []int{1, 2, 3},
			From:       []string{"2.3.4.5/32"},
			To:         []string{"2001:db8::/32"},
			Comment:    "allow apt update",
			wantErr:    true,
			wantErrmsg: "ingress rule with error:mixed address family in one rule is not supported:[2.3.4.5/32 2001:db8::/32]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := IngressRule{
				Protocol: tt.Protocol,
				Ports:    tt.Ports,
				To:       tt.To,
				From:     tt.From,
				Comment:  tt.Comment,
			}
			if err := r.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("IngressRule.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := r.Validate(); err != nil {
				if tt.wantErrmsg != err.Error() {
					t.Errorf("IngressRule.Validate() error = %v, wantErrmsg %v", err.Error(), tt.wantErrmsg)
				}
			}
		})
	}
}
