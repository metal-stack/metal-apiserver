package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
	"github.com/metal-stack/metal-lib/auditing"
	auditingapi "github.com/metal-stack/metal-lib/auditing/api"
	auditingsplunk "github.com/metal-stack/metal-lib/auditing/splunk"
	auditingtimescaledb "github.com/metal-stack/metal-lib/auditing/timescaledb"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/urfave/cli/v2"
)

// might return (nil, nil, nil) if auditing is disabled!
func createAuditingClient(cli *cli.Context, log *slog.Logger) (searchBackend auditingapi.Auditing, backends []auditingapi.Auditing, err error) {
	const (
		auditingBackendTimescaleDB = "timescaledb"
		auditingBackendSplunk      = "splunk"
	)

	var (
		auditingCfg = auditingapi.Config{
			Log:       log,
			Component: api.AuditingComponent,
		}

		timescaledbEnabled = cli.Bool(auditingTimescaleEnabledFlag.Name)
		splunkEnabled      = cli.Bool(auditingSplunkEnabledFlag.Name)

		auditingEnabled = timescaledbEnabled || splunkEnabled
	)

	if !auditingEnabled {
		return nil, nil, nil
	}

	if timescaledbEnabled {
		backend, err := newTimescaledbBackend(cli, auditingCfg)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to initialize timescaledb audit backend: %w", err)
		}

		backends = append(backends, backend)

		log.Info("configured timescaledb audit backend")

		if cli.String(auditingSearchBackendFlag.Name) == auditingBackendTimescaleDB {
			log.Info("using timescaledb audit backend as search backend")
			searchBackend = backend
		}
	}

	if splunkEnabled {
		backend, err := newSplunkBackend(cli, auditingCfg)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to initialize splunk audit backend: %w", err)
		}

		backends = append(backends, backend)

		log.Info("configured splunk audit backend")

		if searchBackend == nil && cli.String(auditingSearchBackendFlag.Name) == auditingBackendSplunk {
			return nil, nil, fmt.Errorf("splunk is not supported as a search backend")
		}
	}

	if backendName := cli.String(auditingSearchBackendFlag.Name); backendName == "" {
		searchBackend = pointer.FirstOrZero(backends)
		log.Info("defaulted audit search backend", "backend-type", fmt.Sprintf("%T", searchBackend))
	} else if searchBackend == nil {
		return nil, nil, fmt.Errorf("search backend not supported or unconfigured: %s", backendName)
	}

	return
}

func newTimescaledbBackend(cli *cli.Context, auditingCfg auditingapi.Config) (searchBackend auditingapi.Auditing, err error) {
	return auditingtimescaledb.NewTimescaleDB(auditingCfg, auditingtimescaledb.TimescaleDbConfig{
		Host:      cli.String(auditingTimescaleHostFlag.Name),
		Port:      cli.String(auditingTimescalePortFlag.Name),
		DB:        cli.String(auditingTimescaleDbFlag.Name),
		User:      cli.String(auditingTimescaleUserFlag.Name),
		Password:  cli.String(auditingTimescalePasswordFlag.Name),
		Retention: cli.String(auditingTimescaleRetentionFlag.Name),
	})
}

func newSplunkBackend(cli *cli.Context, auditingCfg auditingapi.Config) (searchBackend auditingapi.Auditing, err error) {
	const (
		splunkAsyncBackoff = 1 * time.Second
		splunkAsyncRetry   = 3
	)

	host := cli.String(auditingSplunkHostFlag.Name)
	if host == "" {
		host, err = os.Hostname()
		if err != nil {
			return nil, err
		}
	}

	source := cli.App.Name
	if s := cli.String(auditingSplunkSourceFlag.Name); s != "" {
		source = s
	}

	splunkConfig := auditingsplunk.SplunkConfig{
		Endpoint:   cli.String(auditingSplunkEndpointFlag.Name),
		HECToken:   cli.String(auditingSplunkHecTokenFlag.Name),
		SourceType: cli.String(auditingSplunkSourceTypeFlag.Name),
		Index:      cli.String(auditingSplunkIndexFlag.Name),
		Host:       host,
	}

	if caPath := cli.String(auditingSplunkCaFlag.Name); caPath != "" {
		caCert, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("unable to read ca cert: %w", err)
		}

		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		splunkConfig.TlsConfig = &tls.Config{
			RootCAs:    caCertPool,
			MinVersion: tls.VersionTLS12,
		}
	}

	splunkBackend, err := auditingsplunk.NewSplunk(auditingapi.Config{
		Component: source,
		Log:       auditingCfg.Log,
	}, splunkConfig)
	if err != nil {
		return nil, err
	}

	return auditing.NewAsync(splunkBackend, auditingCfg.Log, auditing.AsyncConfig{
		AsyncRetry:   splunkAsyncRetry,
		AsyncBackoff: new(splunkAsyncBackoff),
	})
}
