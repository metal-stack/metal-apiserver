package token

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

const (
	separator = ":"
	prefix    = "tokenstore_"
)

var (
	ErrTokenNotFound = valkey.Nil
)

type TokenStore interface {
	Set(ctx context.Context, token *apiv2.Token) error
	Get(ctx context.Context, userid, tokenid string) (*apiv2.Token, error)
	List(ctx context.Context, userid string) ([]*apiv2.Token, error)
	AdminList(ctx context.Context) ([]*apiv2.Token, error)
	Revoke(ctx context.Context, userid, tokenid string) error
	Migrate(ctx context.Context, log *slog.Logger) error
}

type redisStore struct {
	client valkey.Client
}

func key(userid, tokenid string) string {
	return prefix + userid + separator + tokenid
}

func match(userid string) string {
	return prefix + userid + separator + "*"
}

func NewRedisStore(client valkey.Client) TokenStore {
	return &redisStore{
		client: client,
	}
}

func (r *redisStore) Set(ctx context.Context, token *apiv2.Token) error {
	encoded, err := json.Marshal(toInternal(token))
	if err != nil {
		return fmt.Errorf("unable to encode token: %w", err)
	}

	err = r.client.Do(ctx, r.client.B().Set().Key(key(token.User, token.Uuid)).Value(string(encoded)).ExatTimestamp(token.Expires.AsTime().UnixMilli()).Build()).Error()
	if err != nil {
		return err
	}

	return nil
}

func (r *redisStore) Get(ctx context.Context, userid, tokenid string) (*apiv2.Token, error) {
	encoded, err := r.client.Do(ctx, r.client.B().Get().Key(key(userid, tokenid)).Build()).ToString()
	if err != nil {
		return nil, err
	}

	var t token
	err = json.Unmarshal([]byte(encoded), &t)
	if err != nil {
		return nil, err
	}

	return toExternal(&t), nil
}

func (r *redisStore) List(ctx context.Context, userid string) ([]*apiv2.Token, error) {
	var (
		res []*apiv2.Token
	)
	entry, err := r.client.Do(ctx, r.client.B().Scan().Cursor(0).Match(match(userid)).Build()).AsScanEntry()
	if err != nil {
		return nil, fmt.Errorf("error scanning user token with:%q error:%w", match(userid), err)
	}

	for _, element := range entry.Elements {
		encoded, err := r.client.Do(ctx, r.client.B().Get().Key(element).Build()).AsBytes()
		if err != nil {
			return nil, fmt.Errorf("unable to get content by key:%q error:%w", encoded, err)
		}

		var t token
		err = json.Unmarshal([]byte(encoded), &t)
		if err != nil {
			return nil, fmt.Errorf("unable to decode scan result:%q error:%w", encoded, err)
		}
		res = append(res, toExternal(&t))
	}
	return res, nil
}

func (r *redisStore) AdminList(ctx context.Context) ([]*apiv2.Token, error) {
	var (
		compat = valkeycompat.NewAdapter(r.client)
		res    []*apiv2.Token
	)
	elements, _, err := compat.Scan(ctx, 0, prefix+"*", 0).Result()
	if err != nil {
		return nil, err
	}

	for _, element := range elements {
		encoded, err := r.client.Do(ctx, r.client.B().Get().Key(element).Build()).AsBytes()
		if err != nil {
			return nil, fmt.Errorf("unable to get content by key:%q error:%w", encoded, err)
		}

		var t token
		err = json.Unmarshal([]byte(encoded), &t)
		if err != nil {
			return nil, fmt.Errorf("unable to decode scan result:%q error:%w", encoded, err)
		}
		res = append(res, toExternal(&t))
	}

	return res, nil
}

func (r *redisStore) Revoke(ctx context.Context, userid, tokenid string) error {
	return r.client.Do(ctx, r.client.B().Del().Key(key(userid, tokenid)).Build()).Error()
}

func (r *redisStore) Migrate(ctx context.Context, log *slog.Logger) error {
	tokens, err := r.AdminList(ctx)
	if err != nil {
		return err
	}

	var errs []error

	for range tokens {
		// possible future migrations can go here
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}
