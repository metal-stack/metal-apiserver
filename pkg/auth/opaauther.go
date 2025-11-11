package auth

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/lestrrat-go/jwx/v2/jwk"
	v2 "github.com/metal-stack/api/go/metalstack/api/v2"
	authentication "github.com/metal-stack/metal-apiserver/pkg/auth/authentication"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository"

	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/cache"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	"github.com/open-policy-agent/opa/v1/topdown/print"
)

// TODO check https://github.com/akshayjshah/connectauth for optimization

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

	// opa is a gRPC server authorizer using OPA as backend
	opa struct {
		authenticationQuery *rego.PreparedEvalQuery
		log                 *slog.Logger
		certCache           *cache.Cache[any, *cacheReturn]
		tokenStore          token.TokenStore
	}

	cacheReturn struct {
		raw string
		set jwk.Set
	}

	authenticationDecision struct {
		Valid   bool   `json:"valid"`
		Subject string `json:"subject"`
		JwtID   string `json:"id"`
		Reason  string `json:"reason"`
	}

	printHook struct {
		log *slog.Logger
	}
)

func (p *printHook) Print(ctx print.Context, msg string) error {
	p.log.Debug("rego evaluation", "print output", msg, "print context", ctx)
	return nil
}

// New creates an OPA authorizer
func New(c Config) (*opa, error) {
	var (
		log = c.Log.WithGroup("opa")
		ctx = context.Background()
	)

	authenticationQ, err := newAuthenticationQuery(ctx, log, c.AllowedIssuers)
	if err != nil {
		return nil, err
	}

	certCacheTime := 60 * time.Minute
	if c.CertCacheTime != nil {
		certCacheTime = *c.CertCacheTime
	}

	return &opa{
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
		tokenStore:          c.TokenStore,
		authenticationQuery: &authenticationQ,
	}, nil
}

func newOpaQuery(ctx context.Context, log *slog.Logger, fs embed.FS, query string, data map[string]any) (rego.PreparedEvalQuery, error) {
	files, err := fs.ReadDir(".")
	if err != nil {
		return rego.PreparedEvalQuery{}, err
	}

	var moduleLoads []func(r *rego.Rego)
	for _, f := range files {
		content, err := fs.ReadFile(f.Name())
		if err != nil {
			return rego.PreparedEvalQuery{}, err
		}
		moduleLoads = append(moduleLoads, rego.Module(f.Name(), string(content)))
	}

	moduleLoads = append(moduleLoads, rego.Query(query))
	moduleLoads = append(moduleLoads, rego.EnablePrintStatements(true))
	moduleLoads = append(moduleLoads, rego.PrintHook(&printHook{
		log: log,
	}))
	moduleLoads = append(moduleLoads, rego.Store(inmem.NewFromObject(data)))

	return rego.New(
		moduleLoads...,
	).PrepareForEval(ctx)
}

func newAuthenticationQuery(ctx context.Context, log *slog.Logger, allowedIssuers []string) (rego.PreparedEvalQuery, error) {
	return newOpaQuery(ctx, log, authentication.Policies, "x = data.api.v1.metalstack.io.authentication.decision", map[string]any{
		"allowed_issuers": allowedIssuers,
	})
}

func (o *opa) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return connect.StreamingClientFunc(func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		o.log.Warn("streamclient called", "procedure", spec.Procedure)
		return next(ctx, spec)
	})
}

// WrapStreamingHandler is a Opa StreamServerInterceptor for the
// server. Only one stream interceptor can be installed.
// If you want to add extra functionality you might decorate this function.
func (o *opa) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return connect.StreamingHandlerFunc(func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if o.authenticationQuery == nil {
			return fmt.Errorf("opa engine not initialized properly, forgot AuthzLoad ?")
		}

		wrapper := &recvWrapper{
			StreamingHandlerConn: conn,
			ctx:                  ctx,
			o:                    o,
		}
		return next(ctx, wrapper)
	})
}

type recvWrapper struct {
	connect.StreamingHandlerConn
	ctx context.Context
	o   *opa
}

func (s *recvWrapper) Receive(m any) error {
	if err := s.StreamingHandlerConn.Receive(m); err != nil {
		return err
	}
	_, err := s.o.decide(s.ctx, s.StreamingHandlerConn.Spec().Procedure, s.StreamingHandlerConn.RequestHeader().Get, m)
	if err != nil {
		return err
	}

	return nil
}

// WrapUnary is a Opa UnaryServerInterceptor for the
// server. Only one unary interceptor can be installed.
// If you want to add extra functionality you might decorate this function.
func (o *opa) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	// Same as previous UnaryInterceptorFunc.
	return connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		o.log.Debug("authz unary", "req", req)
		if o.authenticationQuery == nil {
			return nil, fmt.Errorf("opa engine not initialized properly, forgot AuthzLoad ?")
		}
		callinfo, ok := connect.CallInfoForHandlerContext(ctx)
		if !ok {
			return nil, fmt.Errorf("no callinfo in handler context found")
		}
		t, err := o.decide(ctx, req.Spec().Procedure, callinfo.RequestHeader().Get, req)
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

		return resp, err
	})
}

func (o *opa) decide(ctx context.Context, methodName string, jwtTokenfunc func(string) string, req any) (*v2.Token, error) {
	// Allow all methods which have public visibility defined in the proto definition
	// o.log.Debug("authorize", "method", methodName, "req", req, "visibility", o.visibility, "servicepermissions", *o.servicePermissions)

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

	if jwtToken != "" {
		decision, err := o.authenticate(ctx, map[string]any{
			"token": jwtToken,
			"jwks":  jwks.raw,
		})
		if err != nil {
			return nil, errorutil.NewInternal(err)
		}

		if !decision.Valid {
			if decision.Reason != "" {
				return nil, errorutil.NewUnauthenticated(errors.New(decision.Reason))
			}

			return nil, errorutil.Unauthenticated("token is invalid or has expired")
		}

		t, err := o.tokenStore.Get(ctx, decision.Subject, decision.JwtID)
		if err != nil {
			if errors.Is(err, token.ErrTokenNotFound) {
				return nil, errorutil.Unauthenticated("token was revoked")
			}

			return nil, errorutil.NewInternal(err)
		}
		return t, nil
	}

	return nil, nil
}

func (o *opa) authenticate(ctx context.Context, input map[string]any) (authenticationDecision, error) {
	return evalResult[authenticationDecision](ctx, o.log.WithGroup("authentication"), o.authenticationQuery, input)
}

func evalResult[R any](ctx context.Context, log *slog.Logger, query *rego.PreparedEvalQuery, input map[string]any) (R, error) {
	log.Debug("rego query evaluation", "input", input)

	var zero R

	results, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return zero, fmt.Errorf("error evaluating rego result set: %w", err)
	}

	if len(results) == 0 {
		return zero, fmt.Errorf("error evaluating rego result set: result have no length")
	}

	decision, ok := results[0].Bindings["x"].(map[string]any)
	if !ok {
		return zero, fmt.Errorf("error evaluating rego result set: no map contained in decision")
	}

	raw, err := json.Marshal(decision)
	if err != nil {
		return zero, fmt.Errorf("unable to marshal json: %w", err)
	}

	var res R
	err = json.Unmarshal(raw, &res)
	if err != nil {
		return zero, fmt.Errorf("unable to unmarshal json: %w", err)
	}

	// only for devel:
	// log.Debug("made auth decision", "results", results)

	return res, nil
}
