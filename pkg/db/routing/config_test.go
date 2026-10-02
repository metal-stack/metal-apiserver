package routing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Config
		wantErr bool
	}{
		{
			name: "empty means everything on rethink",
			in:   "",
			want: Config{Entities: map[string]Mode{}},
		},
		{
			name: "explicit modes",
			in:   "machine=both,network=postgres",
			want: Config{Entities: map[string]Mode{"machine": ModeBoth, "network": ModePostgres}},
		},
		{
			name: "default and read",
			in:   "default=postgres,read=postgres,machine=both",
			want: Config{Default: ModePostgres, ReadFrom: ModePostgres, Entities: map[string]Mode{"machine": ModeBoth}},
		},
		{
			name: "whitespace is trimmed and keys are case-insensitive",
			in:   " Machine = Both , Network = Postgres ",
			want: Config{Entities: map[string]Mode{"machine": ModeBoth, "network": ModePostgres}},
		},
		{
			name:    "missing value separator",
			in:      "machine",
			wantErr: true,
		},
		{
			name:    "invalid mode",
			in:      "machine=mysql",
			wantErr: true,
		},
		{
			name:    "invalid read backend",
			in:      "read=mysql",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestModeFor(t *testing.T) {
	cfg := Config{
		Default:  ModeRethink,
		ReadFrom: ModePostgres,
		Entities: map[string]Mode{"machine": ModeBoth},
	}

	require.Equal(t, ModeBoth, cfg.ModeFor("machine"))
	require.Equal(t, ModeBoth, cfg.ModeFor("Machine"))
	require.Equal(t, ModeRethink, cfg.ModeFor("network"))

	require.Equal(t, ModeRethink, Config{}.ModeFor("anything"), "zero config keeps rethinkdb")
}

func TestModeForProgrammaticConfig(t *testing.T) {
	cfg := Config{Entities: map[string]Mode{"Machine": ModePostgres}}
	require.Equal(t, ModePostgres, cfg.ModeFor("Machine"))
	require.Equal(t, ModePostgres, cfg.ModeFor("machine"))
}

func TestReadBackend(t *testing.T) {
	cfg := Config{ReadFrom: ModePostgres}

	require.Equal(t, ModeRethink, cfg.readBackend(ModeRethink))
	require.Equal(t, ModePostgres, cfg.readBackend(ModePostgres))
	require.Equal(t, ModePostgres, cfg.readBackend(ModeBoth))
	require.Equal(t, ModeRethink, Config{}.readBackend(ModeBoth), "read defaults to rethinkdb")
}

func TestModeValid(t *testing.T) {
	require.True(t, ModeRethink.Valid())
	require.True(t, ModePostgres.Valid())
	require.True(t, ModeBoth.Valid())
	require.False(t, Mode("mysql").Valid())
}
