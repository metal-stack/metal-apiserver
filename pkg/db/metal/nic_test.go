package metal

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

func TestNicState_SetState(t *testing.T) {
	tests := []struct {
		name        string
		s           *NicState
		status      SwitchPortStatus
		want        *NicState
		wantChanged bool
	}{
		{
			name:   "state is nil",
			s:      nil,
			status: SwitchPortStatusUp,
			want: &NicState{
				Actual: SwitchPortStatusUp,
			},
			wantChanged: true,
		},
		{
			name: "state unchanged and matches desired",
			s: &NicState{
				Desired: pointer.Pointer(SwitchPortStatusUp),
				Actual:  SwitchPortStatusUp,
			},
			status: SwitchPortStatusUp,
			want: &NicState{
				Actual: SwitchPortStatusUp,
			},
			wantChanged: true,
		},
		{
			name: "state unchanged and does not match desired",
			s: &NicState{
				Desired: pointer.Pointer(SwitchPortStatusUp),
				Actual:  SwitchPortStatusDown,
			},
			status: SwitchPortStatusDown,
			want: &NicState{
				Desired: pointer.Pointer(SwitchPortStatusUp),
				Actual:  SwitchPortStatusDown,
			},
			wantChanged: false,
		},
		{
			name: "state changed and desired empty",
			s: &NicState{
				Actual: SwitchPortStatusDown,
			},
			status: SwitchPortStatusUp,
			want: &NicState{
				Actual: SwitchPortStatusUp,
			},
			wantChanged: true,
		},
		{
			name: "state changed and does not match desired",
			s: &NicState{
				Desired: pointer.Pointer(SwitchPortStatusDown),
				Actual:  SwitchPortStatusUnknown,
			},
			status: SwitchPortStatusUp,
			want: &NicState{
				Desired: pointer.Pointer(SwitchPortStatusDown),
				Actual:  SwitchPortStatusUp,
			},
			wantChanged: true,
		},
		{
			name: "state changed and matches desired",
			s: &NicState{
				Desired: pointer.Pointer(SwitchPortStatusDown),
				Actual:  SwitchPortStatusUp,
			},
			status: SwitchPortStatusDown,
			want: &NicState{
				Actual: SwitchPortStatusDown,
			},
			wantChanged: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotState, gotChanged := tt.s.SetState(tt.status)
			if diff := cmp.Diff(tt.want, gotState); diff != "" {
				t.Errorf("NicState.SetState() diff = %v", diff)
			}
			if gotChanged != tt.wantChanged {
				t.Errorf("NicState.SetState() changed = %v, want %v", gotChanged, tt.wantChanged)
			}
		})
	}
}
