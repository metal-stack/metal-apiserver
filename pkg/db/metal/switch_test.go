package metal

import (
	"testing"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
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
			wantErr: false,
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
