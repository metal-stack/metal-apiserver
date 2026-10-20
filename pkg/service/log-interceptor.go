package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect/v2"
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

// Interceptor returns a connect server interceptor that logs every RPC.
func (i *logInterceptor) Interceptor() connect.ServerInterceptor {
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			var (
				log   = i.log.With("procedure", spec.Procedure)
				debug = i.log.Enabled(ctx, slog.LevelDebug)
				start = time.Now()
			)

			if tokenId, ok := tokenId(ctx); ok {
				log = log.With("token", tokenId)
			}

			if debug {
				log.Debug("handling unary call")
			}

			var (
				request  any
				response any
			)

			wrapped := &loggingStream{
				ServerStream: stream,
				onReceive: func(m any) {
					if request == nil {
						request = m
					}
				},
				onSend: func(m any) {
					response = m
				},
			}

			err := next(ctx, spec, wrapped)

			if debug && request != nil {
				log = log.With("request", request)
			}

			log = log.With("duration", time.Since(start).String())

			if debug && response != nil {
				log = log.With("response", response)
			}

			if err != nil {
				log.Error("error during unary call", "error", err)
			}

			log.Info("handled unary call")

			return err
		}
	}
}

type loggingStream struct {
	connect.ServerStream
	onReceive func(any)
	onSend    func(any)
}

func (w *loggingStream) Send(m any) error {
	if w.onSend != nil {
		w.onSend(m)
	}
	return w.ServerStream.Send(m)
}

func (w *loggingStream) Receive(m any) error {
	err := w.ServerStream.Receive(m)
	if err == nil && w.onReceive != nil {
		w.onReceive(m)
	}
	return err
}

const (
	prefix              = "Bearer "
	authorizationHeader = "Authorization"
)

func tokenId(ctx context.Context) (string, bool) {
	callinfo, ok := connect.CallInfoForServerContext(ctx)
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
	_, _, err := jwt.NewParser().ParseUnverified(tokenString, claims)
	if err != nil {
		return "", false
	}

	return claims.ID, true
}
