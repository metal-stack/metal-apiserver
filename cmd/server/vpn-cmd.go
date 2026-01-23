package main

import (
	"fmt"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/headscale"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	vpnadmin "github.com/metal-stack/metal-apiserver/pkg/service/vpn/admin"
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

					hc, err := headscale.NewClient(headscale.Config{
						Log:      log,
						Disabled: !ctx.Bool(headscaleEnabledFlag.Name),
						Apikey:   ctx.String(headscaleApikeyFlag.Name),
						Endpoint: ctx.String(headscaleAddressFlag.Name),
					})
					if err != nil {
						return err
					}
					if hc == nil || !ctx.Bool(headscaleEnabledFlag.Name) {
						log.Info("headscale is disabled, not checking for connected machines")
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

					repo, err := repository.New(log, nil, ds, nil, nil, nil)
					if err != nil {
						return fmt.Errorf("unable to create repository: %w", err)
					}

					vpnService := vpnadmin.New(vpnadmin.Config{
						Log:             log,
						Repo:            repo,
						HeadscaleClient: hc,
					})

					_, err = vpnService.EvaluateVPNConnected(ctx.Context)
					return err
				},
			},
		},
	}
}
