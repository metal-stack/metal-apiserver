package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/headscale"
	"google.golang.org/protobuf/types/known/timestamppb"

	headscalev1 "github.com/juanfont/headscale/gen/go/headscale/v1"
)

const (
	// This policy allows all users to access their own devices.
	// It is suitable for many use cases where you want to
	// allow users to access their own devices, but not other devices in the tailnet.
	HeadscaleDefaultPolicy = `{
		"acls": [
			{
				"action": "accept",
				"src": ["autogroup:member"],
				"dst": ["autogroup:self:*"]
			}
		]
	}`

	defaultExpiration = time.Hour
)

type (
	vpn struct {
		s     *Store
		scope *ProjectScope
		c     *headscale.Client
	}
)

func (v *vpn) Enabled() bool {
	return v.c != nil
}

func (v *vpn) CreateAuthKey(ctx context.Context, req *adminv2.VPNServiceAuthKeyRequest) (*adminv2.VPNServiceAuthKeyResponse, error) {
	projectID := req.Project
	if v.scope != nil {
		projectID = v.scope.projectID
	}
	_, err := v.s.Project(projectID).Get(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	headscaleUser, ok := v.GetUser(ctx, req.Project)
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

	resp, err := v.c.CreatePreAuthKey(ctx, &headscalev1.CreatePreAuthKeyRequest{
		User:       headscaleUser.Id,
		Ephemeral:  req.Ephemeral,
		Expiration: timestamppb.New(expiration),
	})
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return &adminv2.VPNServiceAuthKeyResponse{
		Address:   v.c.Endpoint(),
		AuthKey:   resp.PreAuthKey.Key,
		Ephemeral: resp.PreAuthKey.Ephemeral,
		ExpiresAt: resp.PreAuthKey.Expiration,
		CreatedAt: resp.PreAuthKey.CreatedAt,
	}, nil
}

func (v *vpn) CreateUser(ctx context.Context, name string) (*headscalev1.User, error) {
	resp, err := v.c.CreateUser(ctx, &headscalev1.CreateUserRequest{
		Name: name,
	})
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

func (v *vpn) ListNodes(ctx context.Context, req *adminv2.VPNServiceListNodesRequest) (*adminv2.VPNServiceListNodesResponse, error) {
	lnr := &headscalev1.ListNodesRequest{}
	if req.Project != nil {
		lnr.User = *req.Project
	}

	resp, err := v.c.ListNodes(ctx, lnr)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	var vpnNodes []*apiv2.VPNNode

	for _, node := range resp.Nodes {
		vpnNodes = append(vpnNodes, &apiv2.VPNNode{
			Id:          node.Id,
			Name:        node.Name,
			Project:     node.User.Name,
			IpAddresses: node.IpAddresses,
			LastSeen:    node.LastSeen,
			Online:      node.Online,
		})
	}

	return &adminv2.VPNServiceListNodesResponse{Nodes: vpnNodes}, nil
}

func (v *vpn) GetUser(ctx context.Context, name string) (*headscalev1.User, bool) {
	resp, err := v.c.ListUsers(ctx, &headscalev1.ListUsersRequest{
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

func (v *vpn) ControlPlaneAddress() string {
	return v.c.Endpoint()
}

func (v *vpn) DeleteNode(ctx context.Context, machineID string, projectID string) (*headscalev1.Node, error) {
	machine, err := v.getNode(ctx, machineID, projectID)
	if err != nil {
		return nil, err
	}

	req := &headscalev1.DeleteNodeRequest{
		NodeId: machine.Id,
	}
	if _, err := v.c.DeleteNode(ctx, req); err != nil {
		return nil, fmt.Errorf("failed to delete machine: %w", err)
	}

	return machine, nil
}

func (v *vpn) SetDefaultPolicy(ctx context.Context) error {
	_, err := v.c.SetPolicy(ctx, &headscalev1.SetPolicyRequest{
		Policy: HeadscaleDefaultPolicy,
	})
	if err != nil {
		return err
	}

	return nil
}

func (v *vpn) getNode(ctx context.Context, machineID, projectID string) (machine *headscalev1.Node, err error) {
	req := &headscalev1.ListNodesRequest{
		User: projectID,
	}

	resp, err := v.c.ListNodes(ctx, req)
	if err != nil || resp == nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	for _, m := range resp.Nodes {
		if m.Name == machineID {
			return m, nil
		}
	}

	return nil, errorutil.NotFound("node with id %s and project %s not found", machineID, projectID)
}
