package main

import (
	"fmt"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/headscale"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/vpn"
	"github.com/urfave/cli/v2"
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
		Subcommands: []*cli.Command{
			{
				Name:        "connected-machines",
				Description: "evaluates whether machines connected to vpn and detects their vpn ip addresses",
				Action: func(ctx *cli.Context) error {
					log, err := createLogger(ctx)
					if err != nil {
						return fmt.Errorf("unable to create logger %w", err)
					}

					if !ctx.Bool(headscaleEnabledFlag.Name) {
						log.Info("headscale is disabled, not checking for connected machines")
						return nil
					}

					hc, err := headscale.NewClient(headscale.Config{
						Log:           log,
						Apikey:        ctx.String(headscaleApikeyFlag.Name),
						Endpoint:      ctx.String(headscaleAddressFlag.Name),
						ControllerURL: ctx.String(headscaleControlplaneAddressFlag.Name),
					})
					if err != nil {
						return err
					}

					connectOpts := rethinkdb.ConnectOpts{
						Addresses: ctx.StringSlice(rethinkdbAddressesFlag.Name),
						Database:  ctx.String(rethinkdbDBNameFlag.Name),
						Username:  ctx.String(rethinkdbUserFlag.Name),
						Password:  ctx.String(rethinkdbPasswordFlag.Name),
						MaxIdle:   10,
						MaxOpen:   20,
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

					_, err = vpn.EvaluateVPNConnected(ctx.Context, log, repo)
					return err
				},
			},
		},
	}
}
