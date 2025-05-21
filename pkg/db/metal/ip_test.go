package metal_test

import (
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

func TestIP_GetIPAddress(t *testing.T) {
	tests := []struct {
		name    string
		ip      metal.IP
		want    string
		wantErr bool
	}{
		{
			name: "not namespaced",
			ip: metal.IP{
				IPAddress: "1.2.3.4",
			},
			want:    "1.2.3.4",
			wantErr: false,
		},
		{
			name: "with namespaced",
			ip: metal.IP{
				IPAddress: "aa-bb-cc-1.2.3.4",
				Namespace: pointer.Pointer("aa-bb-cc"),
			},
			want:    "1.2.3.4",
			wantErr: false,
		},
		{
			name: "with namespaced",
			ip: metal.IP{
				IPAddress: "aa-bb-cc-1.2.3.4",
				Namespace: pointer.Pointer("aa-bb-cc-dd"),
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.ip.GetIPAddress()
			if (err != nil) != tt.wantErr {
				t.Errorf("IP.GetIPAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IP.GetIPAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateNamespacedIPAddress(t *testing.T) {
	tests := []struct {
		name      string
		namespace *string
		ip        string
		want      string
	}{
		{
			name:      "no namespace",
			namespace: nil,
			ip:        "1.2.3.4",
			want:      "1.2.3.4",
		},
		{
			name:      "with namespace",
			namespace: pointer.Pointer("aa-bb-cc"),
			ip:        "1.2.3.4",
			want:      "aa-bb-cc-1.2.3.4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := metal.CreateNamespacedIPAddress(tt.namespace, tt.ip); got != tt.want {
				t.Errorf("CreateNamespacedIPAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}
