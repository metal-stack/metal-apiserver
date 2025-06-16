package main

import (
	"context"
	"fmt"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/urfave/cli/v3"
	"gopkg.in/rethinkdb/rethinkdb-go.v6"

	_ "github.com/metal-stack/metal-apiserver/pkg/db/generic/migrations"
)

var (
	targetVersionFlag = &cli.IntFlag{
		Name:  "target-version",
		Value: -1,
		Usage: "the target version of the migration, when set to -1 will migrate to latest version",
	}
	dryRunFlag = &cli.BoolFlag{
		Name:  "dry-run",
		Value: false,
		Usage: "only shows which migrations would run, but does not execute them",
	}
)

func newDatastoreCmd(ctx context.Context) *cli.Command {
	return &cli.Command{
		Name: "datastore",
		Flags: []cli.Flag{
			rethinkdbAddressesFlag,
			rethinkdbDBNameFlag,
			rethinkdbPasswordFlag,
			rethinkdbUserFlag,
			logLevelFlag,
		},
		Commands: []*cli.Command{
			{
				Name:        "init",
				Description: "initializes the datastore. must be run before the server can act on the datastore.",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					log, err := createLogger(cmd)
					if err != nil {
						return fmt.Errorf("unable to create logger %w", err)
					}

					err = generic.Initialize(ctx, log.WithGroup("datastore"), rethinkdb.ConnectOpts{
						Addresses: cmd.StringSlice(rethinkdbAddressesFlag.Name),
						Database:  cmd.String(rethinkdbDBNameFlag.Name),
						Username:  cmd.String(rethinkdbUserFlag.Name),
						Password:  cmd.String(rethinkdbPasswordFlag.Name),
						MaxIdle:   10,
						MaxOpen:   20,
					})
					if err != nil {
						return fmt.Errorf("unable to initialize datastore: %w", err)
					}

					return nil
				},
			},
			{
				Name:        "migrate",
				Description: "migrates the datastore. usually runs at the end of the metal-apiserver rollout. during the migration, the server instances cannot write to the datastore until the migration has finished.",
				Flags: []cli.Flag{
					targetVersionFlag,
					dryRunFlag,
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					log, err := createLogger(cmd)
					if err != nil {
						return fmt.Errorf("unable to create logger %w", err)
					}

					var targetVersion *int
					if v := cmd.Int(targetVersionFlag.Name); v >= 0 {
						targetVersion = &v
					}

					err = generic.Migrate(ctx, rethinkdb.ConnectOpts{
						Addresses: cmd.StringSlice(rethinkdbAddressesFlag.Name),
						Database:  cmd.String(rethinkdbDBNameFlag.Name),
						Username:  cmd.String(rethinkdbUserFlag.Name),
						Password:  cmd.String(rethinkdbPasswordFlag.Name),
						MaxIdle:   10,
						MaxOpen:   20,
					}, log.WithGroup("datastore"), targetVersion, cmd.Bool(dryRunFlag.Name))
					if err != nil {
						return fmt.Errorf("unable to initialize datastore: %w", err)
					}

					return nil
				},
			},
		},
	}
}
