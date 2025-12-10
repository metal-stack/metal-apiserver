package admin

import (
	"context"
	"log/slog"
	"time"

	headscalev1 "github.com/juanfont/headscale/gen/go/headscale/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
)

const defaultExpiration = time.Hour

type Config struct {
	Log              *slog.Logger
	Repo             *repository.Store
	headscaleClient  headscalev1.HeadscaleServiceClient
	headscaleAddress string
}

type vpnService struct {
	log              *slog.Logger
	repo             *repository.Store
	headscaleClient  headscalev1.HeadscaleServiceClient
	headscaleAddress string
}
type VPNService interface {
	adminv2connect.VPNServiceHandler
	CreateUser(context.Context, string) (*headscalev1.User, error)
	UserExists(context.Context, string) (*headscalev1.User, bool)
	ControlPlaneAddress() string
	NodesConnected(context.Context) ([]*headscalev1.Node, error)
	DeleteNode(ctx context.Context, machineID, projectID string) error
}

func New(c Config) VPNService {
	return &vpnService{
		log:              c.Log,
		repo:             c.Repo,
		headscaleClient:  c.headscaleClient,
		headscaleAddress: c.headscaleAddress,
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
		Address: v.headscaleAddress,
		Authkey: key.PreAuthKey.Key,
	}, nil
}

func (v *vpnService) ControlPlaneAddress() string {
	panic("unimplemented")
}

func (v *vpnService) CreateUser(ctx context.Context, name string) (*headscalev1.User, error) {
	resp, err := v.headscaleClient.CreateUser(ctx, &headscalev1.CreateUserRequest{
		Name: name,
	})
	if err != nil {
		return nil, err
	}
	return resp.User, nil
}

func (v *vpnService) DeleteNode(ctx context.Context, machineID string, projectID string) error {
	panic("unimplemented")
}

func (v *vpnService) NodesConnected(context.Context) ([]*headscalev1.Node, error) {
	panic("unimplemented")
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
