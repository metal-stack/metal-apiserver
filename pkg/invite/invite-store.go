package invite

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/valkey-io/valkey-go"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	inviteSecretLength  = 32
	inviteSecretLetters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"

	separator     = ":"
	projectprefix = "invitestore_by_project_"
	tenantprefix  = "invitestore_by_tenant_"
	secretprefix  = "invitestore_by_secret_"
)

var (
	ErrInviteNotFound = valkey.Nil
)

type ProjectInviteStore interface {
	SetInvite(ctx context.Context, invite *apiv2.ProjectInvite) error
	GetInvite(ctx context.Context, secret string) (*apiv2.ProjectInvite, error)
	ListInvites(ctx context.Context, projectID string) ([]*apiv2.ProjectInvite, error)
	DeleteInvite(ctx context.Context, invite *apiv2.ProjectInvite) error
}
type TenantInviteStore interface {
	SetInvite(ctx context.Context, invite *apiv2.TenantInvite) error
	GetInvite(ctx context.Context, secret string) (*apiv2.TenantInvite, error)
	ListInvites(ctx context.Context, login string) ([]*apiv2.TenantInvite, error)
	DeleteInvite(ctx context.Context, invite *apiv2.TenantInvite) error
}

type invite interface {
	GetSecret() string
	GetExpiresAt() *timestamppb.Timestamp
}

type projectRedisStore struct {
	client valkey.Client
}
type tenantRedisStore struct {
	client valkey.Client
}

func NewProjectRedisStore(client valkey.Client) ProjectInviteStore {
	return &projectRedisStore{
		client: client,
	}
}
func NewTenantRedisStore(client valkey.Client) TenantInviteStore {
	return &tenantRedisStore{
		client: client,
	}
}

func projectkey(t *apiv2.ProjectInvite) string {
	return projectprefix + t.Project + separator + t.Secret
}
func tenantkey(t *apiv2.TenantInvite) string {
	return tenantprefix + t.TargetTenant + separator + t.Secret
}

func secretkey(secret string) string {
	return secretprefix + secret
}

func matchProject(projectId string) string {
	return projectprefix + projectId + separator + "*"
}
func matchTenant(tenantId string) string {
	return tenantprefix + tenantId + separator + "*"
}

// Project

func (r *projectRedisStore) SetInvite(ctx context.Context, i *apiv2.ProjectInvite) error {
	return set(ctx, r.client, i, func() string { return projectkey(i) })
}

func (r *projectRedisStore) ListInvites(ctx context.Context, projectid string) ([]*apiv2.ProjectInvite, error) {
	return list[*apiv2.ProjectInvite](ctx, r.client, matchProject(projectid))
}

func (r *projectRedisStore) DeleteInvite(ctx context.Context, i *apiv2.ProjectInvite) error {
	return delete(ctx, r.client, i, func() string { return projectkey(i) })
}

func (r *projectRedisStore) GetInvite(ctx context.Context, secret string) (*apiv2.ProjectInvite, error) {
	return get[*apiv2.ProjectInvite](ctx, r.client, secret)
}

// Tenant

func (r *tenantRedisStore) DeleteInvite(ctx context.Context, i *apiv2.TenantInvite) error {
	return delete(ctx, r.client, i, func() string { return tenantkey(i) })
}

func (r *tenantRedisStore) GetInvite(ctx context.Context, secret string) (*apiv2.TenantInvite, error) {
	return get[*apiv2.TenantInvite](ctx, r.client, secret)
}

func (r *tenantRedisStore) ListInvites(ctx context.Context, tenantID string) ([]*apiv2.TenantInvite, error) {
	return list[*apiv2.TenantInvite](ctx, r.client, matchTenant(tenantID))
}

func (r *tenantRedisStore) SetInvite(ctx context.Context, invite *apiv2.TenantInvite) error {
	return set(ctx, r.client, invite, func() string { return tenantkey(invite) })
}

func get[E any](ctx context.Context, c valkey.Client, secret string) (E, error) {
	var zero E

	if err := validateInviteSecret(secret); err != nil {
		return zero, err
	}

	encoded, err := c.Do(ctx, c.B().Get().Key(secretkey(secret)).Build()).AsBytes()
	if err != nil {
		return zero, fmt.Errorf("unable to get secret as bytes:%w", err)
	}

	var e E
	err = json.Unmarshal(encoded, &e)
	if err != nil {
		return zero, fmt.Errorf("unable to unmarshal secret from bytes:%w", err)
	}

	return e, nil
}

func delete(ctx context.Context, c valkey.Client, i invite, keyFn func() string) error {
	if err := validateInviteSecret(i.GetSecret()); err != nil {
		return err
	}

	cmds := make(valkey.Commands, 0, 2)
	cmds = append(cmds, c.B().Del().Key(secretkey(i.GetSecret())).Build())
	cmds = append(cmds, c.B().Del().Key(keyFn()).Build())
	for i, resp := range c.DoMulti(ctx, cmds...) {
		if resp.Error() != nil {
			return fmt.Errorf("unable delete with command:%s %w", cmds[i].Commands()[1], resp.Error())
		}
	}

	return nil
}

func set(ctx context.Context, c valkey.Client, i invite, keyFn func() string) error {
	if i.GetExpiresAt() == nil {
		return fmt.Errorf("invite needs to have an expiration")
	}

	if err := validateInviteSecret(i.GetSecret()); err != nil {
		return err
	}

	encoded, err := json.Marshal(i)
	if err != nil {
		return fmt.Errorf("unable to encode invite: %w", err)
	}

	cmds := make(valkey.Commands, 0, 2)
	cmds = append(cmds, c.B().Set().Key(keyFn()).Value(string(encoded)).Exat(i.GetExpiresAt().AsTime()).Build())
	cmds = append(cmds, c.B().Set().Key(secretkey(i.GetSecret())).Value(string(encoded)).Exat(i.GetExpiresAt().AsTime()).Build())
	for _, cmd := range cmds {
		fmt.Printf("cmd:%s\n", cmd.Commands())
	}
	for i, resp := range c.DoMulti(ctx, cmds...) {
		if resp.Error() != nil {
			return fmt.Errorf("unable delete with command:%s %w", cmds[i].Commands(), resp.Error())
		}
	}
	return nil
}

func list[E any](ctx context.Context, c valkey.Client, match string) ([]E, error) {
	var (
		res []E
	)
	entry, err := c.Do(ctx, c.B().Scan().Cursor(0).Match(match).Build()).AsScanEntry()
	if err != nil {
		return nil, err
	}

	for _, element := range entry.Elements {
		encoded, err := c.Do(ctx, c.B().Get().Key(element).Build()).AsBytes()
		if err != nil {
			return nil, err
		}

		var i E
		err = json.Unmarshal(encoded, &i)
		if err != nil {
			return nil, err
		}

		res = append(res, i)
	}

	return res, nil
}

// GenerateInviteSecret returns a securely generated random string.
// It will return an error if the system's secure random
// number generator fails to function correctly, in which
// case the caller should not continue.
func GenerateInviteSecret() (string, error) {
	ret := make([]byte, inviteSecretLength)
	for i := range inviteSecretLength {

		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(inviteSecretLetters))))
		if err != nil {
			return "", fmt.Errorf("unable to generate invite secret: %w", err)
		}

		ret[i] = inviteSecretLetters[num.Int64()]
	}

	return string(ret), nil
}

func validateInviteSecret(s string) error {
	if len(s) != inviteSecretLength {
		return fmt.Errorf("unexpected invite secret length")
	}

	for _, letter := range s {
		if !strings.ContainsRune(inviteSecretLetters, letter) {
			return fmt.Errorf("invite secret contains unexpected characters: %s", strconv.QuoteRune(letter))
		}
	}

	return nil
}
