package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/metal-stack/metal-apiserver/pkg/token"
)

type logInterceptor struct {
	log *slog.Logger
}

func newLogRequestInterceptor(log *slog.Logger) *logInterceptor {
	return &logInterceptor{
		log: log,
	}
}

func (i *logInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		var (
			log   = i.log.With("procedure", req.Spec().Procedure)
			debug = i.log.Enabled(ctx, slog.LevelDebug)
			start = time.Now()
		)

		tokenId, ok := tokenId(ctx)
		if ok {
			log = log.With("token", tokenId)
		}

		if debug {
			log = log.With("request", req.Any())
			log.Debug("handling unary call")
		}

		response, err := next(ctx, req)

		log = log.With("duration", time.Since(start).String())

		if debug && response != nil {
			log = log.With("response", response.Any())
		}

		if err != nil {
			log.Error("error during unary call", "error", err)
		}

		log.Info("handled unary call")

		return response, err
	}
}

func (i *logInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		i.log.Warn("streamclient called", "procedure", spec.Procedure)
		return next(ctx, spec)
	}
}

func (i *logInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		wrapper := &wrapper{
			StreamingHandlerConn: conn,
			log:                  i.log,
		}
		return next(ctx, wrapper)
	}
}

type wrapper struct {
	connect.StreamingHandlerConn
	log *slog.Logger
}

func (w *wrapper) Send(m any) error {
	procedure := w.StreamingHandlerConn.Spec().Procedure
	w.log.Debug("streaminghandler send called", "procedure", procedure, "message", m)
	return w.StreamingHandlerConn.Send(m)
}

func (w *wrapper) Receive(m any) error {
	procedure := w.StreamingHandlerConn.Spec().Procedure
	w.log.Debug("streaminghandler receive called", "procedure", procedure, "message", m)
	return w.StreamingHandlerConn.Receive(m)
}

const (
	prefix              = "Bearer "
	authorizationHeader = "Authorization"
)

func tokenId(ctx context.Context) (string, bool) {
	callinfo, ok := connect.CallInfoForHandlerContext(ctx)
	if !ok {
		return "", false
	}
	auth := callinfo.RequestHeader().Get(authorizationHeader)
	// Case insensitive prefix match. See RFC 9110 Section 11.1.
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return "", false
	}
	tokenString := auth[len(prefix):]

	claims := &token.Claims{}
	_, _, err := jwt.NewParser().ParseUnverified(string(tokenString), claims)
	if err != nil {
		return "", false
	}

	return claims.ID, true
}
