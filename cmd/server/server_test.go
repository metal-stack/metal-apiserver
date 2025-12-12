package main

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

// FIXME add more useful tests

func Test_newServeCmd(t *testing.T) {
	app := &cli.App{Writer: io.Discard}
	args := []string{"-h"}

	cmd := newServeCmd()
	require.Len(t, cmd.Flags, 37)

	app.Commands = []*cli.Command{cmd}
	err := app.Run(args)
	require.NoError(t, err)
}

func Test_newDataCmd(t *testing.T) {
	app := &cli.App{Writer: io.Discard}
	args := []string{"-h"}

	cmd := newDatastoreCmd()
	require.Len(t, cmd.Flags, 5)

	app.Commands = []*cli.Command{cmd}
	err := app.Run(args)
	require.NoError(t, err)
}

func Test_newTokenCmd(t *testing.T) {
	app := &cli.App{Writer: io.Discard}
	args := []string{"-h"}

	cmd := newTokenCmd()
	require.Len(t, cmd.Flags, 11)

	app.Commands = []*cli.Command{cmd}
	err := app.Run(args)
	require.NoError(t, err)
}
