package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/lestrrat-go/jwx/v3/jwk"
	v2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"

	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/cache"
)

const (
	authorizationHeader = "authorization"
)

type (
	Config struct {
		Log            *slog.Logger
		CertStore      certs.CertStore
		CertCacheTime  *time.Duration
		TokenStore     token.TokenStore
		AllowedIssuers []string
		Repo           *repository.Store
	}

	// auth is a gRPC server authorizer
	auth struct {
		log           *slog.Logger
		certCache     *cache.Cache[any, *cacheReturn]
		tokenStore    token.TokenStore
		allowedIssuer []string
	}

	cacheReturn struct {
		raw string
		set jwk.Set
	}
)

// NewAuthenticatorInterceptor creates an authenticator
func NewAuthenticatorInterceptor(c Config) (*auth, error) {
	var (
		log = c.Log.WithGroup("auth")
	)

	certCacheTime := 60 * time.Minute
	if c.CertCacheTime != nil {
		certCacheTime = *c.CertCacheTime
	}

	return &auth{
		log: log,
		certCache: cache.New(certCacheTime, func(ctx context.Context, id any) (*cacheReturn, error) {
			set, raw, err := c.CertStore.PublicKeys(ctx)
			if err != nil {
				return nil, fmt.Errorf("unable to retrieve signing certs: %w", err)
			}
			return &cacheReturn{
				set: set,
				raw: raw,
			}, nil
		}),
		tokenStore:    c.TokenStore,
		allowedIssuer: c.AllowedIssuers,
	}, nil
}

func (o *auth) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	}
}

// WrapStreamingHandler is a StreamServerInterceptor for the
// server. Only one stream interceptor can be installed.
// If you want to add extra functionality you might decorate this function.
func (o *auth) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		wrapper := &wrapper{
			StreamingHandlerConn: conn,
			ctx:                  ctx,
			o:                    o,
		}
		return next(ctx, wrapper)
	}
}

type wrapper struct {
	connect.StreamingHandlerConn
	ctx context.Context
	o   *auth
}

func (s *wrapper) Receive(m any) error {
	if err := s.StreamingHandlerConn.Receive(m); err != nil {
		return err
	}

	_, err := s.o.extractAndValidateJWTToken(s.ctx, s.StreamingHandlerConn.RequestHeader().Get)
	if err != nil {
		return err
	}

	return nil
}

// Enable only if response debugging is required
//
// func (s *wrapper) Send(m any) error {
// 	s.o.log.Debug("streaminghandler send called", "message", m)
// 	if err := s.StreamingHandlerConn.Send(m); err != nil {
// 		return err
// 	}

// 	return nil
// }

// WrapUnary is a UnaryServerInterceptor for the
// server. Only one unary interceptor can be installed.
// If you want to add extra functionality you might decorate this function.
func (o *auth) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	// Same as previous UnaryInterceptorFunc.
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		o.log.Debug("authz unary", "req", req)
		callinfo, ok := connect.CallInfoForHandlerContext(ctx)
		if !ok {
			return nil, fmt.Errorf("no callinfo in handler context found")
		}
		t, err := o.extractAndValidateJWTToken(ctx, callinfo.RequestHeader().Get)
		if err != nil {
			return nil, err
		}

		// Store the token in the context for later use in the service methods
		if t != nil {
			ctx = token.ContextWithToken(ctx, t)
		}

		resp, err := next(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("unable to process request %w", err)
		}

		return resp, nil
	}
}

func (o *auth) extractAndValidateJWTToken(ctx context.Context, jwtTokenfunc func(string) string) (*v2.Token, error) {
	jwks, err := o.certCache.Get(ctx, nil)
	if err != nil {
		return nil, err
	}

	if jwks.set.Len() == 0 {
		// in the initial startup phase it can happen that authorize gets called even if there are no public signing keys yet
		// in this case due to caching there is no possibility to authenticate for 60 minutes until the cache has expired
		// so we refresh the cache if nothing was found.
		jwks, err = o.certCache.Refresh(ctx, nil)
		if err != nil {
			return nil, err
		}
	}

	var (
		bearer         = jwtTokenfunc(authorizationHeader)
		_, jwtToken, _ = strings.Cut(bearer, " ")
	)

	jwtToken = strings.TrimSpace(jwtToken)
	o.log.Debug("decide", "jwt", jwtToken)
	if jwtToken == "" {
		return nil, nil
	}

	claim, err := token.Validate(ctx, o.log, jwtToken, jwks.set, o.allowedIssuer)
	if err != nil {
		return nil, errorutil.NewUnauthenticated(err)
	}

	t, err := o.tokenStore.Get(ctx, claim.Subject, claim.ID)
	if err != nil {
		if errors.Is(err, token.ErrTokenNotFound) {
			return nil, errorutil.Unauthenticated("token was revoked")
		}

		return nil, errorutil.NewInternal(err)
	}
	return t, nil
}
