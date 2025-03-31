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
			name:       "verify validation of input fails",
			acquire:    524288,
			acquireErr: errors.New("value '524288' is outside of the allowed range '1 - 131072'"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := ds.AsnPool().AcquireUniqueInteger(t.Context(), tt.acquire)
			if tt.acquireErr != nil {
				require.EqualError(t, err, tt.acquireErr.Error())
				return
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.acquire, value)
			}
			got := ds.AsnPool().ReleaseUniqueInteger(t.Context(), tt.release)
			if tt.releaseErr != nil {
				require.EqualError(t, got, tt.releaseErr.Error())
			} else {
				require.NoError(t, got)
			}

		})
	}
}
