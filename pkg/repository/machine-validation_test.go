package repository

import (
	"errors"
	"testing"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/stretchr/testify/assert"
)

func Test_validateFirewallSpec(t *testing.T) {
	tests := []struct {
		name         string
		firewallSpec *apiv2.FirewallSpec
		wantErr      error
	}{
		{
			name: "valid firewall rules",
			firewallSpec: &apiv2.FirewallSpec{
				FirewallRules: &apiv2.FirewallRules{
					Egress: []*apiv2.FirewallEgressRule{
						{
							Protocol: apiv2.IPProtocol_IP_PROTOCOL_TCP,
							Ports:    []uint32{22, 443, 8080},
							To:       []string{"0.0.0.0/0"},
							Comment:  "outgoing traffic allowed",
						},
						{
							Protocol: apiv2.IPProtocol_IP_PROTOCOL_TCP,
							Ports:    []uint32{22, 443, 8080},
							To:       []string{"::/0"},
							Comment:  "outgoing ipvsix traffic allowed",
						},
					},
					Ingress: []*apiv2.FirewallIngressRule{
						{
							Protocol: apiv2.IPProtocol_IP_PROTOCOL_TCP,
							Ports:    []uint32{443},
							To:       []string{"10.0.0.1/32"},
							From:     []string{"0.0.0.0/0"},
							Comment:  "webserver exposal",
						},
						{
							Protocol: apiv2.IPProtocol_IP_PROTOCOL_TCP,
							Ports:    []uint32{443},
							From:     []string{"::/0"},
							Comment:  "webserver ipvsix exposal",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "invalid firewall rules, unknown protocol",
			firewallSpec: &apiv2.FirewallSpec{
				FirewallRules: &apiv2.FirewallRules{
					Egress: []*apiv2.FirewallEgressRule{
						{
							Protocol: apiv2.IPProtocol_IP_PROTOCOL_UNSPECIFIED,
							Ports:    []uint32{22, 443, 8080},
							To:       []string{"0.0.0.0/0"},
							Comment:  "outgoing traffic allowed",
						},
					},
				},
			},
			wantErr: errors.New("unable to fetch stringvalue from IP_PROTOCOL_UNSPECIFIED"),
		},
		{
			name: "invalid firewall rules, port out of range",
			firewallSpec: &apiv2.FirewallSpec{
				FirewallRules: &apiv2.FirewallRules{
					Egress: []*apiv2.FirewallEgressRule{
						{
							Protocol: apiv2.IPProtocol_IP_PROTOCOL_UDP,
							Ports:    []uint32{22, 443, 8080, 100000},
							To:       []string{"0.0.0.0/0"},
							Comment:  "outgoing traffic allowed",
						},
					},
				},
			},
			wantErr: errors.New("egress rule with error:port 100000 is out of range"),
		},
		{
			name: "invalid firewall rules, invalid mask",
			firewallSpec: &apiv2.FirewallSpec{
				FirewallRules: &apiv2.FirewallRules{
					Egress: []*apiv2.FirewallEgressRule{
						{
							Protocol: apiv2.IPProtocol_IP_PROTOCOL_UDP,
							Ports:    []uint32{22, 443, 8080},
							To:       []string{"0.0.0.0/33"},
							Comment:  "outgoing traffic allowed",
						},
					},
				},
			},
			wantErr: errors.New(`egress rule with error:invalid cidr: netip.ParsePrefix("0.0.0.0/33"): prefix length out of range`),
		},
		{
			name: "invalid firewall rules, invalid comment",
			firewallSpec: &apiv2.FirewallSpec{
				FirewallRules: &apiv2.FirewallRules{
					Egress: []*apiv2.FirewallEgressRule{
						{
							Protocol: apiv2.IPProtocol_IP_PROTOCOL_UDP,
							Ports:    []uint32{22, 443, 8080},
							To:       []string{"0.0.0.0/32"},
							Comment:  "0 numbers are not allowed",
						},
					},
				},
			},
			wantErr: errors.New(`egress rule with error:illegal character '0' in comment found, only: "abcdefghijklmnopqrstuvwxyz_- " allowed`),
		},
		{
			name: "invalid firewall rules, mixed addressfamilies",
			firewallSpec: &apiv2.FirewallSpec{
				FirewallRules: &apiv2.FirewallRules{
					Ingress: []*apiv2.FirewallIngressRule{
						{
							Protocol: apiv2.IPProtocol_IP_PROTOCOL_UDP,
							Ports:    []uint32{22, 443, 8080},
							From:     []string{"0.0.0.0/32"},
							To:       []string{"::/0"},
							Comment:  "mixed af",
						},
					},
				},
			},
			wantErr: errors.New(`ingress rule with error:mixed address family in one rule is not supported:[0.0.0.0/32 ::/0]`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFirewallSpec(tt.firewallSpec)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
		})
	}
}
