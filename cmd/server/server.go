package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"

	taskserver "github.com/metal-stack/metal-apiserver/pkg/async/task/server"
	"github.com/metal-stack/metal-apiserver/pkg/service"
)

type server struct {
	c   service.Config
	log *slog.Logger
}

func newServer(c service.Config) *server {
	return &server{
		c:   c,
		log: c.Log,
	}
}

func (s *server) Run(ctx context.Context) error {
	mux, err := service.New(s.log, s.c)
	if err != nil {
		return err
	}

	p := new(http.Protocols)
	p.SetHTTP1(true)
	// For gRPC clients, it's convenient to support HTTP/2 without TLS.
	p.SetUnencryptedHTTP2(true)

	apiServer := &http.Server{
		Addr:              s.c.HttpServerEndpoint,
		Handler:           newCORS().Handler(mux),
		Protocols:         p,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		MaxHeaderBytes:    8 * 1024, // 8KiB
	}
	s.log.Info("serving http on", "addr", apiServer.Addr)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		if err := apiServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("HTTP listen and serve", "error", err)
			os.Exit(1)
		}
	}()

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	ms := &http.Server{
		Addr:              s.c.MetricsServerEndpoint,
		Handler:           metricsMux,
		ReadHeaderTimeout: time.Minute,
	}
	go func() {
		s.log.Info("serving metrics at", "addr", ms.Addr+"/metrics")
		err := ms.ListenAndServe()
		if err != nil {
			s.log.Error("unable to start metric endpoint", "error", err)
			return
		}
	}()

	taskServer, taskServerMux := taskserver.NewServer(s.log, s.c.Repository, s.c.RedisConfig.AsyncClient)
	go func() {
		s.log.Info("starting asynq server")
		if err := taskServer.Run(taskServerMux); err != nil {
			s.log.Error("unable to start asynq server", "error", err)
			return
		}
	}()

	<-signals
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	taskServer.Shutdown()
	return apiServer.Shutdown(ctx)
}

// newCORS
// FIXME replace with https://github.com/connectrpc/cors-go
func newCORS() *cors.Cors {
	// To let web developers play with the demo service from browsers, we need a
	// very permissive CORS setup.
	return cors.New(cors.Options{
		AllowedMethods: []string{
			http.MethodHead,
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		},
		AllowOriginFunc: func(origin string) bool {
			// Allow all origins, which effectively disables CORS.
			return true
		},
		AllowedHeaders: []string{"*"},
		ExposedHeaders: []string{
			// Content-Type is in the default safelist.
			"Accept",
			"Accept-Encoding",
			"Accept-Post",
			"Connect-Accept-Encoding",
			"Connect-Content-Encoding",
			"Connect-Protocol-Version",
			"Content-Encoding",
			"Grpc-Accept-Encoding",
			"Grpc-Encoding",
			"Grpc-Message",
			"Grpc-Status",
			"Grpc-Status-Details-Bin",
		},
		// Let browsers cache CORS information for longer, which reduces the number
		// of preflight requests. Any changes to ExposedHeaders won't take effect
		// until the cached data expires. FF caps this value at 24h, and modern
		// Chrome caps it at 2h.
		MaxAge: int(2 * time.Hour / time.Second),
	})
}
