package main

import (
	"context"
	"fmt"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/headscale"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/vpn"
	"github.com/urfave/cli/v3"
	"gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func newVPNCmd() *cli.Command {
	return &cli.Command{
		Name: "vpn",
		Flags: []cli.Flag{
			rethinkdbAddressesFlag,
			rethinkdbDBNameFlag,
			rethinkdbPasswordFlag,
			rethinkdbUserFlag,
			headscaleAddressFlag,
			headscaleApikeyFlag,
			headscaleEnabledFlag,
			headscaleControlplaneAddressFlag,
		},
		Commands: []*cli.Command{
			{
				Name:        "connected-machines",
				Description: "evaluates whether machines connected to vpn and detects their vpn ip addresses",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					log, err := createLogger(cmd)
					if err != nil {
						return fmt.Errorf("unable to create logger %w", err)
					}

					if !cmd.Bool(headscaleEnabledFlag.Name) {
						log.Info("headscale is disabled, not checking for connected machines")
						return nil
					}

					hc, err := headscale.NewClient(headscale.Config{
						Log:           log,
						Apikey:        cmd.String(headscaleApikeyFlag.Name),
						Endpoint:      cmd.String(headscaleAddressFlag.Name),
						ControllerURL: cmd.String(headscaleControlplaneAddressFlag.Name),
					})
					if err != nil {
						return err
					}

					connectOpts := rethinkdb.ConnectOpts{
						Addresses:  cmd.StringSlice(rethinkdbAddressesFlag.Name),
						Database:   cmd.String(rethinkdbDBNameFlag.Name),
						Username:   cmd.String(rethinkdbUserFlag.Name),
						Password:   cmd.String(rethinkdbPasswordFlag.Name),
						InitialCap: 10,
						MaxOpen:    20,
					}

					ds, err := generic.New(log.WithGroup("datastore"), connectOpts)
					if err != nil {
						return fmt.Errorf("unable to create datastore: %w", err)
					}

					repo := repository.New(repository.Config{
						Log:             log,
						Datastore:       ds,
						HeadscaleClient: hc,
					})

					_, err = vpn.EvaluateVPNConnected(ctx, log, repo)
					return err
				},
			},
		},
	}
}
