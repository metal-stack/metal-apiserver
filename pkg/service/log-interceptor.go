package service

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
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
	return connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		var (
			log   = i.log.With("procedure", req.Spec().Procedure)
			debug = i.log.Enabled(ctx, slog.LevelDebug)
		)

		if debug {
			log = log.With("request", req.Any())
		}

		log.Info("handling unary call")

		response, err := next(ctx, req)
		if err != nil {
			if debug && response != nil {
				log = log.With("response", response.Any())
			}

			log.Error("error during unary call", "error", err)
		}

		return response, err
	})
}

func (i *logInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return connect.StreamingClientFunc(func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return next(ctx, spec)
	})
}

func (i *logInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return connect.StreamingHandlerFunc(func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(ctx, conn)
	})
}
