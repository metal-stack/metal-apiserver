package repository

import (
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

func Test_checkDuplicateNics(t *testing.T) {
	type args struct {
		nics metal.Nics
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkDuplicateNics(tt.args.nics); (err != nil) != tt.wantErr {
				t.Errorf("checkDuplicateNics() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
