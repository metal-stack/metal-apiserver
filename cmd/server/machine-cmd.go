package main

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	compress "github.com/klauspost/connect-compress/v2"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/headscale"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/vpn"
	"github.com/urfave/cli/v3"
	"gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func newMachineCmd() *cli.Command {
	return &cli.Command{
		Name:        "machine",
		Description: "evaluates whether machines liveliness or resurrect dead machines",
		Commands: []*cli.Command{
			{
				Name:        "liveliness",
				Description: "evaluates whether machines are still alive or if they have died",
				Flags: []cli.Flag{
					rethinkdbAddressesFlag,
					rethinkdbDBNameFlag,
					rethinkdbPasswordFlag,
					rethinkdbUserFlag,
					tenantApiserverBaseURLFlag,
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					log, err := createLogger(cmd)
					if err != nil {
						return fmt.Errorf("unable to create logger %w", err)
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

					tc, err := createTenantApiserverClient(cmd, log)
					if err != nil {
						return err
					}

					repo := repository.New(repository.Config{
						Log:                   log,
						Datastore:             ds,
						TenantApiserverClient: tc,
					})

					return repo.UnscopedMachine().AdditionalMethods().MachineLiveliness(ctx)
				},
			},
			{
				Name:        "resurrect",
				Description: "resurrect dead machines",
				Flags: []cli.Flag{
					headscaleAddressFlag,
					headscaleApikeyFlag,
					headscaleEnabledFlag,
					headscaleControlplaneAddressFlag,
					ipamGrpcEndpointFlag,
					rethinkdbAddressesFlag,
					rethinkdbDBNameFlag,
					rethinkdbPasswordFlag,
					rethinkdbUserFlag,
					tenantApiserverBaseURLFlag,
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					log, err := createLogger(cmd)
					if err != nil {
						return fmt.Errorf("unable to create logger %w", err)
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

					hc, err := headscale.NewClient(headscale.Config{
						Log:           log,
						Apikey:        cmd.String(headscaleApikeyFlag.Name),
						Endpoint:      cmd.String(headscaleAddressFlag.Name),
						ControllerURL: cmd.String(headscaleControlplaneAddressFlag.Name),
					})
					if err != nil {
						return err
					}

					ipamgrpcendpoint := cmd.String(ipamGrpcEndpointFlag.Name)
					ipamService := ipamv1connect.NewIpamServiceClient(
						http.DefaultClient,
						ipamgrpcendpoint,
						connect.WithGRPC(),
						compress.WithAll(compress.LevelBalanced),
					)

					tc, err := createTenantApiserverClient(cmd, log)
					if err != nil {
						return err
					}

					repo := repository.New(repository.Config{
						Log:                   log,
						Datastore:             ds,
						TenantApiserverClient: tc,
						HeadscaleClient:       hc,
						Ipam:                  ipamService,
					})

					return repo.UnscopedMachine().AdditionalMethods().Resurrect(ctx)
				},
			},
			{
				Name:        "vpn-connected",
				Description: "evaluates whether machines connected to vpn and detects their vpn ip addresses",
				Flags: []cli.Flag{
					headscaleAddressFlag,
					headscaleApikeyFlag,
					headscaleEnabledFlag,
					headscaleControlplaneAddressFlag,
					ipamGrpcEndpointFlag,
					rethinkdbAddressesFlag,
					rethinkdbDBNameFlag,
					rethinkdbPasswordFlag,
					rethinkdbUserFlag,
					tenantApiserverBaseURLFlag,
				},
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

					tc, err := createTenantApiserverClient(cmd, log)
					if err != nil {
						return err
					}

					repo := repository.New(repository.Config{
						Log:                   log,
						Datastore:             ds,
						TenantApiserverClient: tc,
						HeadscaleClient:       hc,
					})

					_, err = vpn.EvaluateVPNConnected(ctx, log, repo)
					return err
				},
			},
		},
	}
}
