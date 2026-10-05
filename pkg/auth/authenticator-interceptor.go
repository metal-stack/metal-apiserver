package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/metal-stack/api/go/errorutil"
	v2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/certs"

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

func (o *auth) Authenticate(ctx context.Context, spec connect.Spec, peer connect.Peer, header http.Header) (context.Context, error) {
	log := o.log.With("procedure", spec.Procedure, "peer", peer.Addr)
	log.Debug("authenticate")
	t, err := o.extractAndValidateJWTToken(ctx, header.Get)
	if err != nil {
		log.Error("authenticate, access denied")
		return ctx, err
	}
	// Store the token in the context for later use in the service methods
	if t != nil {
		ctx = token.ContextWithToken(ctx, t)
	}
	return ctx, nil
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
		if errorutil.IsNotFound(err) {
			return nil, errorutil.Unauthenticated("token was revoked")
		}

		return nil, err
	}
	return t, nil
}
