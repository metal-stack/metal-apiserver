package main

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// FIXME add more useful tests

func Test_newServeCmd(t *testing.T) {
	app := &cli.Command{Writer: io.Discard}
	args := []string{"-h"}

	cmd := newServeCmd()
	require.Len(t, cmd.Flags, 48)

	app.Commands = []*cli.Command{cmd}
	err := app.Run(context.Background(), args)
	require.NoError(t, err)
}

func Test_newDataCmd(t *testing.T) {
	app := &cli.Command{Writer: io.Discard}
	args := []string{"-h"}

	cmd := newDatastoreCmd()
	require.Len(t, cmd.Flags, 9)

	app.Commands = []*cli.Command{cmd}
	err := app.Run(context.Background(), args)
	require.NoError(t, err)
}

func Test_newTokenCmd(t *testing.T) {
	app := &cli.Command{Writer: io.Discard}
	args := []string{"-h"}

	cmd := newTokenCmd()
	require.Len(t, cmd.Flags, 17)

	app.Commands = []*cli.Command{cmd}
	err := app.Run(context.Background(), args)
	require.NoError(t, err)
}
