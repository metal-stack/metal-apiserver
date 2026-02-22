package generic

import (
	"reflect"
	"testing"
)

func TestMigrations_Between(t *testing.T) {
	tests := []struct {
		name    string
		ms      migrations
		current int
		target  *int
		want    migrations
		wantErr bool
	}{
		{
			name:    "no migrations is fine",
			ms:      []Migration{},
			current: 0,
			want:    nil,
			wantErr: false,
		},
		{
			name: "get all migrations from 0, sorted",
			ms: []Migration{
				{
					Name:    "migration 4",
					Version: 4,
				},
				{
					Name:    "migration 2",
					Version: 2,
				},
				{
					Name:    "migration 1",
					Version: 1,
				},
			},
			current: 0,
			want: []Migration{
				{
					Name:    "migration 1",
					Version: 1,
				},
				{
					Name:    "migration 2",
					Version: 2,
				},
				{
					Name:    "migration 4",
					Version: 4,
				},
			},
			wantErr: false,
		},
		{
			name: "get all migrations from 1, sorted",
			ms: []Migration{
				{
					Name:    "migration 4",
					Version: 4,
				},
				{
					Name:    "migration 2",
					Version: 2,
				},
				{
					Name:    "migration 1",
					Version: 1,
				},
			},
			current: 1,
			want: []Migration{
				{
					Name:    "migration 2",
					Version: 2,
				},
				{
					Name:    "migration 4",
					Version: 4,
				},
			},
			wantErr: false,
		},
		{
			name: "get migrations up to target version, sorted",
			ms: []Migration{
				{
					Name:    "migration 4",
					Version: 4,
				},
				{
					Name:    "migration 2",
					Version: 2,
				},
				{
					Name:    "migration 1",
					Version: 1,
				},
			},
			current: 0,
			target:  new(2),
			want: []Migration{
				{
					Name:    "migration 1",
					Version: 1,
				},
				{
					Name:    "migration 2",
					Version: 2,
				},
			},
			wantErr: false,
		},
		{
			name: "error on unknown target version",
			ms: []Migration{
				{
					Name:    "migration 4",
					Version: 4,
				},
				{
					Name:    "migration 2",
					Version: 2,
				},
				{
					Name:    "migration 1",
					Version: 1,
				},
			},
			current: 0,
			target:  new(3),
			want:    nil,
			wantErr: true,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.ms.between(tt.current, tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("between() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("between() = %v, want %v", got, tt.want)
			}
		})
	}
}
