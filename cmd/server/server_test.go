package main

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// FIXME add more useful tests

func Test_newServeCmd(t *testing.T) {
	app := &cli.Command{Writer: io.Discard}
	args := []string{"-h"}

	cmd := newServeCmd(t.Context())
	require.Len(t, cmd.Flags, 35)

	app.Commands = []*cli.Command{cmd}
	err := app.Run(t.Context(), args)
	require.NoError(t, err)
}

func Test_newDataCmd(t *testing.T) {
	app := &cli.Command{Writer: io.Discard}
	args := []string{"-h"}

	cmd := newDatastoreCmd(t.Context())
	require.Len(t, cmd.Flags, 5)

	app.Commands = []*cli.Command{cmd}
	err := app.Run(t.Context(), args)
	require.NoError(t, err)
}

func Test_newTokenCmd(t *testing.T) {
	app := &cli.Command{Writer: io.Discard}
	args := []string{"-h"}

	cmd := newTokenCmd(t.Context())
	require.Len(t, cmd.Flags, 11)

	app.Commands = []*cli.Command{cmd}
	err := app.Run(t.Context(), args)
	require.NoError(t, err)
}
