package vpn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/samber/lo"
)

func EvaluateVPNConnected(ctx context.Context, log *slog.Logger, repo *repository.Store) ([]*apiv2.Machine, error) {
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
