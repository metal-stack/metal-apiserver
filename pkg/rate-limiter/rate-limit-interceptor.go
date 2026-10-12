package ratelimiter

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect/v2"
	"github.com/metal-stack/api/go/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/token"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Log         *slog.Logger
	RedisClient *redis.Client

	MaxRequestsPerMinuteToken           int
	MaxRequestsPerMinuteUnauthenticated int
}

type ratelimitInterceptor struct {
	ratelimiter                         *ratelimiter
	maxRequestsPerMinuteToken           int
	maxRequestsPerMinuteUnauthenticated int
	log                                 *slog.Logger
}

func NewInterceptor(c *Config) *ratelimitInterceptor {
	return &ratelimitInterceptor{
		ratelimiter:                         New(c.RedisClient),
		maxRequestsPerMinuteToken:           c.MaxRequestsPerMinuteToken,
		maxRequestsPerMinuteUnauthenticated: c.MaxRequestsPerMinuteUnauthenticated,
		log:                                 c.Log,
	}
}

// Interceptor checks if the rate limit for the given token is raised.
func (i *ratelimitInterceptor) Interceptor() connect.ServerInterceptor {
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			var (
				err   error
				t, ok = token.TokenFromContext(ctx)
			)

			if ok && t != nil {
				_, err = i.ratelimiter.CheckLimitTokenAccess(ctx, t, i.maxRequestsPerMinuteToken)
			} else {
				info, _ := connect.CallInfoForServerContext(ctx)

				clientIP, ok := extractClientIP(info.RequestHeader())
				if !ok {
					i.log.Warn("not finding original client ip in forwarding header, skipping rate-limiter for unauthenticated api access. please configure the ingress-controller to contain the original client ip address in the header.")
					return next(ctx, spec, stream)
				}

				_, err = i.ratelimiter.CheckLimitUnauthenticatedAccess(ctx, clientIP, i.maxRequestsPerMinuteUnauthenticated)
			}

			if err != nil {
				if ratelimiterError, ok := errors.AsType[*errRatelimitReached](err); ok {
					return errorutil.NewResourceExhausted(ratelimiterError)
				}
				return errorutil.NewInternal(err)
			}

			return next(ctx, spec, stream)
		}
	}
}

func extractClientIP(header *connect.Header) (string, bool) {
	ip := header.Get("X-Forwarded-For")
	if ip != "" {
		return ip, true
	}

	ip = header.Get("X-Real-Ip")
	if ip != "" {
		return ip, true
	}

	return "", false
}
