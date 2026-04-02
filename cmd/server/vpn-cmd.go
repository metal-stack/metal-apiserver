package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/headscale"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/samber/lo"
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

					repo := repository.New(repository.Config{
						Log:             log,
						Datastore:       ds,
						HeadscaleClient: hc,
					})

					_, err = evaluateVPNConnected(ctx.Context, log, repo)
					return err
				},
			},
		},
	}
}

func evaluateVPNConnected(ctx context.Context, log *slog.Logger, repo *repository.Store) ([]*apiv2.Machine, error) {
	ms, err := repo.UnscopedMachine().List(ctx, &apiv2.MachineQuery{
		Allocation: &apiv2.MachineAllocationQuery{
			// Return only allocated machines which have a vpn configured
			Vpn: &apiv2.MachineVPN{},
		},
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	listNodesResp, err := repo.UnscopedVPN().ListNodes(ctx, &adminv2.VPNServiceListNodesRequest{})
	if err != nil {
		return nil, err
	}

	var (
		errs            []error
		updatedMachines []*apiv2.Machine
	)
	for _, m := range ms {
		if m.Allocation == nil || m.Allocation.Vpn == nil {
			continue
		}

		node, ok := lo.Find(listNodesResp.Nodes, func(node *apiv2.VPNNode) bool {
			return node.Name == m.Uuid && node.Project == m.Allocation.Project
		})
		if !ok {
			continue
		}

		connected := node.Online
		ips := node.IpAddresses

		if m.Allocation.Vpn.Connected == connected && slices.Equal(m.Allocation.Vpn.Ips, ips) {
			log.Info("not updating vpn because already up-to-date", "machine", m.Uuid, "connected", connected, "ips", ips)
			continue
		}

		updatedMachine, err := repo.UnscopedMachine().AdditionalMethods().SetMachineConnectedToVPN(ctx, m.Uuid, connected, ips)
		if err != nil {
			errs = append(errs, err)
			log.Error("unable to update vpn connected state, continue anyway", "machine", m.Uuid, "error", err)
			continue
		}

		updatedMachines = append(updatedMachines, updatedMachine)
		log.Info("updated vpn connected state", "machine", m.Uuid, "connected", connected, "ips", ips)
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("errors occurred when evaluating machine vpn connections:%w", errors.Join(errs...))
	}

	return updatedMachines, nil
}
