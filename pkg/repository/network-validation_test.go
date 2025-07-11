package repository

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

func Test_networkRepository_validateChildPrefixLength(t *testing.T) {
	tests := []struct {
		name     string
		cpl      metal.ChildPrefixLength
		prefixes metal.Prefixes
		wantErr  error
	}{
		{
			name: "child prefix length contains ipv4 address family that does not exist in prefixes",
			cpl: metal.ChildPrefixLength{
				metal.AddressFamilyIPv4: 28,
			},
			prefixes: metal.Prefixes{
				{IP: "2001::", Length: "64"},
			},
			wantErr: fmt.Errorf(`child prefix length for addressfamily "IPv4" specified, but not found in prefixes`),
		},
		{
			name: "child prefix length contains ipv6 address family that does not exist in prefixes",
			cpl: metal.ChildPrefixLength{
				metal.AddressFamilyIPv6: 56,
			},
			prefixes: metal.Prefixes{
				{IP: "192.168.2.0", Length: "24"},
			},
			wantErr: fmt.Errorf(`child prefix length for addressfamily "IPv6" specified, but not found in prefixes`),
		},
		{
			name: "ipv4 child prefix length is too small",
			cpl: metal.ChildPrefixLength{
				metal.AddressFamilyIPv4: 12,
			},
			prefixes: metal.Prefixes{
				{IP: "192.168.2.0", Length: "24"},
			},
			wantErr: fmt.Errorf("given childprefixlength 12 is not greater than prefix length of:192.168.2.0/24"),
		},
		{
			name: "ipv4 child prefix length is equal, which does not work either",
			cpl: metal.ChildPrefixLength{
				metal.AddressFamilyIPv4: 24,
			},
			prefixes: metal.Prefixes{
				{IP: "192.168.2.0", Length: "24"},
			},
			wantErr: fmt.Errorf("given childprefixlength 24 is not greater than prefix length of:192.168.2.0/24"),
		},
		{
			name: "ipv4 child prefix length is greater and works",
			cpl: metal.ChildPrefixLength{
				metal.AddressFamilyIPv4: 25,
			},
			prefixes: metal.Prefixes{
				{IP: "192.168.2.0", Length: "24"},
			},
			wantErr: nil,
		},
		{
			name: "ipv4 child prefix length must be greater than all given prefixes",
			cpl: metal.ChildPrefixLength{
				metal.AddressFamilyIPv4: 12,
			},
			prefixes: metal.Prefixes{
				{IP: "10.0.0.0", Length: "8"},
				{IP: "192.168.2.0", Length: "24"},
			},
			wantErr: fmt.Errorf("given childprefixlength 12 is not greater than prefix length of:192.168.2.0/24"),
		},
		{
			name: "ipv6 child prefix length is too small",
			cpl: metal.ChildPrefixLength{
				metal.AddressFamilyIPv6: 48,
			},
			prefixes: metal.Prefixes{
				{IP: "2001::", Length: "64"},
			},
			wantErr: fmt.Errorf("given childprefixlength 48 is not greater than prefix length of:2001::/64"),
		},
		{
			name: "errors for both families",
			cpl: metal.ChildPrefixLength{
				metal.AddressFamilyIPv4: 12,
				metal.AddressFamilyIPv6: 48,
			},
			prefixes: metal.Prefixes{
				{IP: "10.0.0.0", Length: "16"},
				{IP: "2001::", Length: "64"},
			},
			wantErr: fmt.Errorf("given childprefixlength 12 is not greater than prefix length of:10.0.0.0/16\ngiven childprefixlength 48 is not greater than prefix length of:2001::/64"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &networkRepository{}

			err := n.validateChildPrefixLength(tt.cpl, tt.prefixes)

			if err == nil {
				err = errors.New("")
			}
			if tt.wantErr == nil {
				tt.wantErr = errors.New("")
			}

			if diff := cmp.Diff(err.Error(), tt.wantErr.Error()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}

func Test_networkRepository_validatePrefixesOnBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		prefixes []string
		wantErr  error
	}{
		{
			name:     "correct ipv4",
			prefixes: []string{"10.0.0.0/8", "192.168.1.0/24"},
		},
		{
			name:     "one incorrect ipv4",
			prefixes: []string{"10.0.0.0/8", "192.168.1.0/24", "10.105.0.0/14"},
			wantErr:  errors.New(`expecting canonical form of prefix "10.105.0.0/14", please specify it as "10.104.0.0/14"`),
		},
		{
			name:     "two incorrect ipv4",
			prefixes: []string{"20.105.0.0/14", "10.0.0.0/8", "192.168.1.0/24", "10.105.0.0/14"},
			wantErr: errors.New(`expecting canonical form of prefix "20.105.0.0/14", please specify it as "20.104.0.0/14"
expecting canonical form of prefix "10.105.0.0/14", please specify it as "10.104.0.0/14"`),
		},
		{
			name:     "correct ipv6",
			prefixes: []string{"2001:abcd:1::/96"},
		},
		{
			name:     "one incorrect ipv6",
			prefixes: []string{"2001:abcd:1::/96", "2001:abcd:1:1::/48"},
			wantErr:  errors.New(`expecting canonical form of prefix "2001:abcd:1:1::/48", please specify it as "2001:abcd:1::/48"`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &networkRepository{}
			err := n.validatePrefixesOnBoundaries(tt.prefixes)

			if err == nil {
				err = errors.New("")
			}
			if tt.wantErr == nil {
				tt.wantErr = errors.New("")
			}

			if diff := cmp.Diff(err.Error(), tt.wantErr.Error()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}
