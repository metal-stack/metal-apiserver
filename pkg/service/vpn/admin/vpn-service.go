package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	headscalev1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-lib/pkg/pointer"
)

const defaultExpiration = time.Hour

type Config struct {
	Log                          *slog.Logger
	Repo                         *repository.Store
	HeadscaleClient              headscalev1.HeadscaleServiceClient
	HeadscaleControlplaneAddress string
}

type vpnService struct {
	log                          *slog.Logger
	repo                         *repository.Store
	headscaleClient              headscalev1.HeadscaleServiceClient
	headscaleControlplaneAddress string
}

type VPNService interface {
	adminv2connect.VPNServiceHandler
	// CreateUser creates a user which maps to a metal project in headscale
	CreateUser(context.Context, string) (*headscalev1.User, error)
	// DeleteNode deletes a node in headscale
	DeleteNode(ctx context.Context, machineID, projectID string) (*headscalev1.Node, error)
	// EvaluateVPNConnected iterates over all connected nodes and
	// updates the machines with the online status in the vpn and their vpn ip adressess
	// It returns the updated machines, machines which already have the correct
	// online status and ip adressess are not touched
	EvaluateVPNConnected(ctx context.Context) ([]*apiv2.Machine, error)
	// ControlPlaneAddress returns the address of headscale where tailscale clients must connect to
	ControlPlaneAddress() string
	// SetDefaultPolicy stores a acl which allows communication between machines in the same project only
	// Should be called on startup
	SetDefaultPolicy() error
}

func New(c Config) VPNService {
	return &vpnService{
		log:                          c.Log,
		repo:                         c.Repo,
		headscaleClient:              c.HeadscaleClient,
		headscaleControlplaneAddress: c.HeadscaleControlplaneAddress,
	}
}

func (v *vpnService) Authkey(ctx context.Context, req *adminv2.VPNServiceAuthkeyRequest) (*adminv2.VPNServiceAuthkeyResponse, error) {
	_, err := v.repo.Project(req.Project).Get(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	headscaleUser, ok := v.userExists(ctx, req.Project)
	if !ok {
		user, err := v.CreateUser(ctx, req.Project)
		if err != nil {
			return nil, err
		}
		headscaleUser = user
	}

	expiration := time.Now()
	if req.Expires != nil {
		expiration = expiration.Add(req.Expires.AsDuration())
	} else {
		expiration = expiration.Add(defaultExpiration)
	}
	key, err := v.headscaleClient.CreatePreAuthKey(ctx, &headscalev1.CreatePreAuthKeyRequest{
		User:       headscaleUser.Id,
		Ephemeral:  req.Ephemeral,
		Expiration: timestamppb.New(expiration),
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.VPNServiceAuthkeyResponse{
		Address: v.headscaleControlplaneAddress,
		Authkey: key.PreAuthKey.Key,
	}, nil
}

func (v *vpnService) ControlPlaneAddress() string {
	return v.headscaleControlplaneAddress
}

func (v *vpnService) CreateUser(ctx context.Context, name string) (*headscalev1.User, error) {
	resp, err := v.headscaleClient.CreateUser(ctx, &headscalev1.CreateUserRequest{
		Name: name,
	})
	// TODO check if this is still like this
	if err != nil {
		// Importing the error from "github.com/juanfont/headscale/hscontrol/db" would pull
		// the whole headscale dependencies and the resulting binary would be ~10Mb bigger
		if strings.Contains(err.Error(), "user already exists") || strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, errorutil.NewConflict(err)
		}
		return nil, err
	}
	return resp.User, nil
}

func (v *vpnService) DeleteNode(ctx context.Context, machineID string, projectID string) (*headscalev1.Node, error) {
	machine, err := v.getNode(ctx, machineID, projectID)
	if err != nil {
		return nil, err
	}

	req := &headscalev1.DeleteNodeRequest{
		NodeId: machine.Id,
	}
	if _, err := v.headscaleClient.DeleteNode(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to delete machine: %w", err)
	}

	return machine, nil
}

// ListNodes implements [VPNService].
func (v *vpnService) ListNodes(ctx context.Context, req *adminv2.VPNServiceListNodesRequest) (*adminv2.VPNServiceListNodesResponse, error) {
	lnr := &headscalev1.ListNodesRequest{}
	if req.User != nil {
		lnr.User = *req.User
	}
	resp, err := v.headscaleClient.ListNodes(ctx, lnr)
	if err != nil {
		return nil, fmt.Errorf("failed to list machines: %w", err)
	}
	var vpnNodes []*apiv2.VPNNode
	for _, node := range resp.Nodes {
		vpnNodes = append(vpnNodes, &apiv2.VPNNode{
			Id:          node.Id,
			Name:        node.Name,
			User:        &node.User.Name,
			IpAddresses: node.IpAddresses,
			LastSeen:    node.LastSeen,
			Online:      node.Online,
		})
	}

	return &adminv2.VPNServiceListNodesResponse{Nodes: vpnNodes}, nil
}

func (v *vpnService) userExists(ctx context.Context, name string) (*headscalev1.User, bool) {
	resp, err := v.headscaleClient.ListUsers(ctx, &headscalev1.ListUsersRequest{
		Name: name,
	})
	if err != nil {
		return nil, false
	}
	var headscaleUser *headscalev1.User
	for _, user := range resp.Users {
		if user.Name == name {
			headscaleUser = user
		}
	}
	if headscaleUser == nil {
		return nil, false
	}
	return headscaleUser, true
}

func (v *vpnService) getNode(ctx context.Context, machineID, projectID string) (machine *headscalev1.Node, err error) {
	req := &headscalev1.ListNodesRequest{
		User: projectID,
	}
	resp, err := v.headscaleClient.ListNodes(ctx, req)
	if err != nil || resp == nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	v.log.Debug("getNode", "nodes", resp.Nodes)
	for _, m := range resp.Nodes {
		if m.Name == machineID {
			return m, nil
		}
	}

	return nil, errorutil.NotFound("node with id %s and project %s not found", machineID, projectID)
}

func (v *vpnService) EvaluateVPNConnected(ctx context.Context) ([]*apiv2.Machine, error) {
	ms, err := v.repo.UnscopedMachine().List(ctx, &apiv2.MachineQuery{
		Allocation: &apiv2.MachineAllocationQuery{
			// Return only allocation machines which have a vpn configured
			Vpn: &apiv2.MachineVPN{},
		},
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	listNodesResp, err := v.ListNodes(ctx, &adminv2.VPNServiceListNodesRequest{})
	if err != nil {
		return nil, err
	}

	v.log.Debug("evaluated vpn connected", "machines", ms, "nodes", listNodesResp.Nodes)

	var (
		errs            []error
		updatedMachines []*apiv2.Machine
	)
	for _, m := range ms {
		if m.Allocation == nil || m.Allocation.Vpn == nil {
			continue
		}

		index := slices.IndexFunc(listNodesResp.Nodes, func(node *apiv2.VPNNode) bool {
			if node.Name != m.Uuid {
				return false
			}

			if pointer.SafeDeref(node.User) != m.Allocation.Project {
				return false
			}

			return true
		})

		if index < 0 {
			continue
		}

		connected := listNodesResp.Nodes[index].Online
		ips := listNodesResp.Nodes[index].IpAddresses

		if m.Allocation.Vpn.Connected == connected && slices.Equal(m.Allocation.Vpn.Ips, ips) {
			v.log.Info("not updating vpn because already up-to-date", "machine", m.Uuid, "connected", connected, "ips", ips)
			continue
		}

		updatedMachine, err := v.repo.UnscopedMachine().AdditionalMethods().SetMachineConnectedToVPN(ctx, m.Uuid, connected, ips)
		if err != nil {
			errs = append(errs, err)
			v.log.Error("unable to update vpn connected state, continue anyway", "machine", m.Uuid, "error", err)
			continue
		}

		updatedMachines = append(updatedMachines, updatedMachine)
		v.log.Info("updated vpn connected state", "machine", m.Uuid, "connected", connected, "ips", ips)
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("errors occurred when evaluating machine vpn connections:%w", errors.Join(errs...))
	}

	return updatedMachines, nil
}

// This policy allows all users to access their own devices.
// It is suitable for many use cases where you want to
// allow users to access their own devices, but not other devices in the tailnet.
const defaultPolicy = `{
		"acls": [
			{
				"action": "accept",
				"src": ["autogroup:member"],
				"dst": ["autogroup:self:*"]
			}
		]
	}`

func (v *vpnService) SetDefaultPolicy() error {
	resp, err := v.headscaleClient.SetPolicy(context.Background(), &headscalev1.SetPolicyRequest{
		Policy: defaultPolicy,
	})
	if err != nil {
		return err
	}
	v.log.Info("setdefaultpolicy", "policy stored", resp.Policy)
	return nil
}
