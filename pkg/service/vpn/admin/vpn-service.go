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
	headscaledb "github.com/juanfont/headscale/hscontrol/db"
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
	CreateUser(context.Context, string) (*headscalev1.User, error)
	UserExists(context.Context, string) (*headscalev1.User, bool)
	ControlPlaneAddress() string
	NodesConnected(context.Context) ([]*headscalev1.Node, error)
	DeleteNode(ctx context.Context, machineID, projectID string) error
	EvaluateVPNConnected(ctx context.Context) error
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

	headscaleUser, ok := v.UserExists(ctx, req.Project)
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
	if err != nil && !strings.Contains(err.Error(), headscaledb.ErrUserExists.Error()) {
		return nil, fmt.Errorf("failed to create new VPN user: %w", err)
	}
	return resp.User, nil
}

func (v *vpnService) DeleteNode(ctx context.Context, machineID string, projectID string) error {
	machine, err := v.getNode(ctx, machineID, projectID)
	if err != nil || machine == nil {
		return err
	}

	req := &headscalev1.DeleteNodeRequest{
		NodeId: machine.Id,
	}
	if _, err := v.headscaleClient.DeleteNode(ctx, req); err != nil {
		return fmt.Errorf("failed to delete machine: %w", err)
	}

	return nil
}

func (v *vpnService) NodesConnected(ctx context.Context) ([]*headscalev1.Node, error) {
	resp, err := v.headscaleClient.ListNodes(ctx, &headscalev1.ListNodesRequest{})
	if err != nil || resp == nil {
		return nil, fmt.Errorf("failed to list machines: %w", err)
	}

	return resp.Nodes, nil
}

func (v *vpnService) UserExists(ctx context.Context, name string) (*headscalev1.User, bool) {
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

	for _, m := range resp.Nodes {
		if m.Name == machineID {
			return m, nil
		}
	}

	return nil, nil
}

func (v *vpnService) EvaluateVPNConnected(ctx context.Context) error {
	ms, err := v.repo.UnscopedMachine().List(ctx, &apiv2.MachineQuery{
		Allocation: &apiv2.MachineAllocationQuery{
			// Return only allocation machines which have a vpn configured
			Vpn: &apiv2.MachineVPN{},
		},
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	headscaleNodes, err := v.NodesConnected(ctx)
	if err != nil {
		return err
	}

	var errs []error
	for _, m := range ms {
		m := m
		if m.Allocation == nil || m.Allocation.Vpn == nil {
			continue
		}

		index := slices.IndexFunc(headscaleNodes, func(hm *headscalev1.Node) bool {
			if hm.Name != m.Uuid {
				return false
			}

			if pointer.SafeDeref(hm.User).Name != m.Allocation.Project {
				return false
			}

			return true
		})

		if index < 0 {
			continue
		}

		connected := headscaleNodes[index].Online
		ips := headscaleNodes[index].IpAddresses

		if m.Allocation.Vpn.Connected == connected && slices.Equal(m.Allocation.Vpn.Ips, ips) {
			v.log.Info("not updating vpn because already up-to-date", "machine", m.Uuid, "connected", connected, "ips", ips)
			continue
		}

		err = v.repo.UnscopedMachine().AdditionalMethods().SetMachineConnectedToVPN(ctx, m.Uuid, connected, ips)
		if err != nil {
			errs = append(errs, err)
			v.log.Error("unable to update vpn connected state, continue anyway", "machine", m.Uuid, "error", err)
			continue
		}

		v.log.Info("updated vpn connected state", "machine", m.Uuid, "connected", connected, "ips", ips)
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors occurred when evaluating machine vpn connections:%w", errors.Join(errs...))
	}

	return nil
}
