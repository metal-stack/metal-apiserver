package token

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/redis/go-redis/v9"
)

const (
	separator = ":"
	prefix    = "tokenstore_"
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
	client *redis.Client
}

func key(userid, tokenid string) string {
	return prefix + userid + separator + tokenid
}

func match(userid string) string {
	return prefix + userid + separator + "*"
}

func NewRedisStore(client *redis.Client) TokenStore {
	return &redisStore{
		client: client,
	}
}

func (r *redisStore) Set(ctx context.Context, token *apiv2.Token) error {
	encoded, err := json.Marshal(toInternal(token))
	if err != nil {
		return errorutil.Internal("unable to encode token: %w", err)
	}

	if token.Meta == nil {
		token.Meta = &apiv2.Meta{}
	}

	// TODO: implement optimistic locking. when using the valkey client, they advise to use a lua script for this.
	// token.Meta.UpdatedAt = timestamppb.Now()

	if token.Meta != nil && token.Meta.UpdatedAt != nil {
		return errorutil.InvalidArgument("optimistic locking is not yet implemented, please do not provide updated_at")
	}

	_, err = r.client.Set(ctx, key(token.User, token.Uuid), string(encoded), time.Until(token.Expires.AsTime())).Result()
	if err != nil {
		return errorutil.Internal("unable to set token: %w", err)
	}

	return nil
}

func (r *redisStore) Get(ctx context.Context, userid, tokenid string) (*apiv2.Token, error) {
	encoded, err := r.client.Get(ctx, key(userid, tokenid)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, errorutil.NotFound("token not found")
		}

		return nil, errorutil.Internal("unable to get token: %w", err)
	}

	var t token
	err = json.Unmarshal([]byte(encoded), &t)
	if err != nil {
		return nil, errorutil.Internal("unable to decode token: %w", err)
	}

	return toExternal(&t), nil
}

func (r *redisStore) List(ctx context.Context, userid string) ([]*apiv2.Token, error) {
	var (
		res  []*apiv2.Token
		iter = r.client.Scan(ctx, 0, match(userid), 0).Iterator()
	)

	for iter.Next(ctx) {
		encoded, err := r.client.Get(ctx, iter.Val()).Result()
		if err != nil {
			return nil, errorutil.Internal("unable to get token: %w", err)
		}

		var t token
		err = json.Unmarshal([]byte(encoded), &t)
		if err != nil {
			return nil, errorutil.Internal("unable to decode token: %w", err)
		}

		res = append(res, toExternal(&t))
	}
	if err := iter.Err(); err != nil {
		return nil, errorutil.Internal("unable to iterate tokens: %w", err)
	}

	return res, nil
}

func (r *redisStore) AdminList(ctx context.Context) ([]*apiv2.Token, error) {
	var (
		res  []*apiv2.Token
		iter = r.client.Scan(ctx, 0, prefix+"*", 0).Iterator()
	)

	for iter.Next(ctx) {
		encoded, err := r.client.Get(ctx, iter.Val()).Result()
		if err != nil {
			return nil, errorutil.Internal("unable to get token: %w", err)
		}

		var t token
		err = json.Unmarshal([]byte(encoded), &t)
		if err != nil {
			return nil, errorutil.Internal("unable to decode token: %w", err)
		}

		res = append(res, toExternal(&t))
	}
	if err := iter.Err(); err != nil {
		return nil, errorutil.Internal("unable to iterate tokens: %w", err)
	}

	return res, nil
}

func (r *redisStore) Revoke(ctx context.Context, userid, tokenid string) error {
	res, err := r.client.Del(ctx, key(userid, tokenid)).Result()
	if err != nil {
		return errorutil.Internal("unable to revoke token: %w", err)
	}

	if res == 0 {
		return errorutil.NotFound("token not found")
	}

	return nil
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
