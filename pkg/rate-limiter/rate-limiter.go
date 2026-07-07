package ratelimiter

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

const (
	separator = ":"
	prefix    = "ratelimiter_"
	halfHour  = 30 * time.Minute
)

type (
	errRatelimitReached struct {
		limit int
	}

	ratelimiter struct {
		client valkey.Client
	}
)

func New(client valkey.Client) *ratelimiter {
	return &ratelimiter{
		client: client,
	}
}

// CheckLimitTokenAccess enforces maxRequestsPerMinute for the given token
func (r *ratelimiter) CheckLimitTokenAccess(ctx context.Context, t *apiv2.Token, maxRequestsPerMinute int) (bool, error) {
	if token.IsAdminToken(t) {
		// admin tokens should not have a rate-limit (i.e. the accounting uses the api excessively to report usages)
		return true, nil
	}

	return r.limit(ctx, keyFromToken(t), maxRequestsPerMinute)
}

// CheckLimitTokenAccess enforces maxRequestsPerMinute for the given token
func (r *ratelimiter) CheckLimitUnauthenticatedAccess(ctx context.Context, ip string, maxRequestsPerMinute int) (bool, error) {
	return r.limit(ctx, keyFromIP(ip), maxRequestsPerMinute)
}

func (r *ratelimiter) limit(ctx context.Context, k string, maxRequestsPerMinute int) (bool, error) {
	count, err := r.client.Do(ctx, r.client.B().Get().Key(k).Build()).AsInt64()
	if err != nil && !errors.Is(err, valkey.Nil) {
		return false, err
	}

	if err == nil {
		if count > int64(maxRequestsPerMinute) {
			return false, &errRatelimitReached{limit: maxRequestsPerMinute}
		}
	}

	cmds := make(valkey.Commands, 0, 2)
	cmds = append(cmds, r.client.B().Incr().Key(k).Build())
	cmds = append(cmds, r.client.B().Expire().Key(k).Seconds(int64(halfHour.Seconds())).Build())
	for i, resp := range r.client.DoMulti(ctx, cmds...) {
		if resp.Error() != nil {
			return false, fmt.Errorf("unable to increment rate-limit count with command:%s %w", cmds[i].Commands()[1], resp.Error())
		}
	}

	return true, nil
}

func keyFromToken(t *apiv2.Token) string {
	return prefix + t.User + separator + t.Uuid + separator + strconv.Itoa(time.Now().Minute())
}

func keyFromIP(ip string) string {
	return prefix + ip + separator + strconv.Itoa(time.Now().Minute())
}

// Error implements the error interface
func (e *errRatelimitReached) Error() string {
	return fmt.Sprintf("you have reached the per-minute API rate limit (limit: %d)", e.limit)
}

// Unwrap implements the errorsas interface
func (e *errRatelimitReached) Unwrap() error {
	return nil
}
