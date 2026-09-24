package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	infrav2connect "github.com/metal-stack/api/go/metalstack/infra/v2/infrav2connect"
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

func Test_mustWithoutStreamWriteDeadline(t *testing.T) {
	t.Parallel()

	const (
		streamPath   = infrav2connect.BMCServiceWaitForBMCCommandProcedure
		unaryPath    = infrav2connect.BMCServiceBMCCommandDoneProcedure
		writeTimeout = 100 * time.Millisecond
		writeDelay   = 300 * time.Millisecond
	)

	handler := mustWithoutStreamWriteDeadline(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(writeDelay)
		_, _ = w.Write([]byte("ok"))
	}))

	ts := httptest.NewUnstartedServer(handler)
	ts.Config.WriteTimeout = writeTimeout
	ts.Start()
	defer ts.Close()

	resp, err := http.Post(ts.URL+streamPath, "application/octet-stream", nil)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "ok", string(body))

	_, err = http.Post(ts.URL+unaryPath, "application/octet-stream", nil)
	require.Error(t, err)
}
