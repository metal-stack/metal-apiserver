package health

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"connectrpc.com/connect"
	apiv1 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/metalstack/api/v2/apiv2connect"
	ipamv1connect "github.com/metal-stack/go-ipam/api/v1/apiv1connect"
	mdm "github.com/metal-stack/masterdata-api/pkg/client"
	"golang.org/x/sync/errgroup"
	"gopkg.in/rethinkdb/rethinkdb-go.v6"
)

const (
	CheckerTimeout = 10 * time.Second
)

type healthchecker interface {
	Health(context.Context) *apiv1.HealthStatus
}

type Config struct {
	Log                  *slog.Logger
	Ctx                  context.Context
	HealthcheckInterval  time.Duration
	Ipam                 ipamv1connect.IpamServiceClient
	Masterdata           mdm.Client
	RethinkDBConnectOpts *rethinkdb.ConnectOpts
}

type healthServiceServer struct {
	log *slog.Logger

	checkers []healthchecker
	current  *apiv1.Health
}

func New(c Config) (apiv2connect.HealthServiceHandler, error) {
	var checkers []healthchecker

	if c.Ipam != nil {
		checkers = append(checkers, &ipamHealthChecker{ipam: c.Ipam})
	}
	if c.Masterdata != nil {
		checkers = append(checkers, &masterdataHealthChecker{mdm: c.Masterdata})
	}
	if c.RethinkDBConnectOpts != nil {
		checkers = append(checkers, &rethinkdbHealthChecker{connectOpts: *c.RethinkDBConnectOpts})
	}
	h := &healthServiceServer{
		log: c.Log.WithGroup("healthService"),
		// initializing status with healthy at the start
		// --> at the beginning we always assume healthy state
		current:  newHealthyServiceMap(),
		checkers: checkers,
	}

	go h.fetchStatuses(c.Ctx, c.HealthcheckInterval)

	return h, nil
}

func (h *healthServiceServer) Get(ctx context.Context, rq *connect.Request[apiv1.HealthServiceGetRequest]) (*connect.Response[apiv1.HealthServiceGetResponse], error) {
	return connect.NewResponse(&apiv1.HealthServiceGetResponse{
		Health: h.current,
	}), nil
}

func (h *healthServiceServer) fetchStatuses(ctx context.Context, interval time.Duration) {
	err := h.updateStatuses(ctx)
	if err != nil {
		h.log.Error("service statuses cannot be fetched, status not updated", "error", err)
	}

	var (
		lastUpdate = time.Now()
		ticker     = time.NewTicker(interval)
	)

	for {
		select {
		case <-ticker.C:
			if time.Since(lastUpdate) < CheckerTimeout {
				h.log.Info("skip updating health status because last update was happening lately")
				continue
			}

			err := h.updateStatuses(ctx)
			if err != nil {
				h.log.Error("service statuses cannot be fetched, status not updated", "error", err)
			}

			lastUpdate = time.Now()

		case <-ctx.Done():
			h.log.Info("stopping health service status fetching")
			ticker.Stop()
			return
		}
	}
}

func (h *healthServiceServer) updateStatuses(outerCtx context.Context) error {
	var (
		statuses        = &apiv1.Health{}
		ctx, cancel     = context.WithTimeout(outerCtx, CheckerTimeout)
		group, groupCtx = errgroup.WithContext(ctx)
		resultChan      = make(chan *apiv1.HealthStatus)
		once            sync.Once
	)

	defer cancel()
	defer once.Do(func() { close(resultChan) })

	for _, checker := range h.checkers {
		if checker == nil {
			continue
		}

		checker := checker

		group.Go(func() error {
			resultChan <- checker.Health(groupCtx)
			return nil
		})
	}

	finished := make(chan bool)
	go func() {
		for r := range resultChan {
			r := r
			statuses.Services = append(statuses.Services, r)
		}

		finished <- true
	}()

	if err := group.Wait(); err != nil {
		return err
	}

	once.Do(func() { close(resultChan) })

	<-finished

	sort.Slice(statuses.Services, func(i, j int) bool {
		return statuses.Services[i].Name < statuses.Services[j].Name
	})

	h.current = statuses

	h.log.Info("health statuses checked successfully")

	return nil
}

func newHealthyServiceMap() *apiv1.Health {
	h := &apiv1.Health{}
	for i := range apiv1.Service_name {
		if i == 0 {
			// skipping unspecified
			continue
		}
		h.Services = append(h.Services, &apiv1.HealthStatus{
			Name:    apiv1.Service(i),
			Status:  apiv1.ServiceStatus_SERVICE_STATUS_HEALTHY,
			Message: "",
		})
	}

	sort.Slice(h.Services, func(i, j int) bool {
		return h.Services[i].Name < h.Services[j].Name
	})

	return h
}
