package service

import (
	"context"
	"log/slog"
	"time"

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
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		var (
			log   = i.log.With("procedure", req.Spec().Procedure)
			debug = i.log.Enabled(ctx, slog.LevelDebug)
			start = time.Now()
		)

		if debug {
			log = log.With("request", req.Any())
		}

		log.Info("handling unary call")

		response, err := next(ctx, req)

		if debug && response != nil {
			log = log.With("response", response.Any())
		}

		if err != nil {
			log.Error("error during unary call", "error", err)
		} else if debug {
			log.Debug("handled call successfully", "duration", time.Since(start).String())
		}

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
