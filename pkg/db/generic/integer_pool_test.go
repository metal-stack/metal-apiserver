package generic_test

import (
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/stretchr/testify/require"
)

// func Test_makeRange(t *testing.T) {
// 	type args struct {
// 		min uint
// 		max uint
// 	}
// 	tests := []struct {
// 		name string
// 		args args
// 		want []integer
// 	}{
// 		{
// 			name: "verify minimum range 1-1",
// 			args: args{min: 1, max: 1},
// 			want: []integer{{1}},
// 		},
// 		{
// 			name: "verify range 1-5",
// 			args: args{min: 1, max: 5},
// 			want: []integer{{1}, {2}, {3}, {4}, {5}},
// 		},
// 		{
// 			name: "verify empty range on max less than min",
// 			args: args{min: 1, max: 0},
// 			want: []integer{},
// 		},
// 		{
// 			name: "verify zero result",
// 			args: args{min: 0, max: 0},
// 			want: []integer{{0}},
// 		},
// 	}
// 	for i := range tests {
// 		tt := tests[i]
// 		t.Run(tt.name, func(t *testing.T) {
// 			if got := makeRange(tt.args.min, tt.args.max); !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("makeRange() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

func Test_AcquireAndReleaseUniqueInteger(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	container, c, err := test.StartRethink(t, log)
	require.NoError(t, err)
	defer func() {
		_ = container.Terminate(t.Context())
	}()

	ds, err := generic.New(log, c)
	require.NoError(t, err)

	tests := []struct {
		name       string
		acquire    uint
		release    uint
		acquireErr error
		releaseErr error
	}{
		{
			name:       "10 succeeds",
			acquire:    10,
			release:    10,
			acquireErr: nil,
		},
		{
			name:       "verify deletion fails",
			acquire:    9,
			release:    8,
			releaseErr: errors.New("any error    message indicating insert failed"),
		},
		{
			name:       "verify validation of input fails",
			acquire:    524288,
			acquireErr: errors.New("value '524288' is outside of the allowed range '1 - 131072'"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := ds.AsnPool().AcquireUniqueInteger(tt.acquire)
			if tt.acquireErr != nil {
				require.EqualError(t, err, tt.acquireErr.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.acquire, value)
			}
			got := ds.AsnPool().ReleaseUniqueInteger(tt.acquire)
			if tt.releaseErr != nil {
				require.EqualError(t, got, tt.releaseErr.Error())
			} else {
				require.NoError(t, got)
			}

		})
	}
}
