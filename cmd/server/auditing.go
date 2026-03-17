package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-lib/auditing"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/urfave/cli/v2"
)

// might return (nil, nil) if auditing is disabled!
func createAuditingClient(cli *cli.Context, log *slog.Logger) (searchBackend auditing.Auditing, backends []auditing.Auditing, err error) {
	const (
		auditingBackendTimescaleDB = "timescaledb"
		auditingBackendSplunk      = "splunk"

		splunkAsyncBackoff = 1 * time.Second
		splunkAsyncRetry   = 3
	)

	var (
		timescaledbEnabled = cli.Bool(auditingTimescaleEnabledFlag.Name)
		splunkEnabled      = cli.Bool(auditingSplunkEnabledFlag.Name)

		auditingEnabled = timescaledbEnabled || splunkEnabled
	)

	if !auditingEnabled {
		return nil, nil, nil
	}

	auditingCfg := auditing.Config{
		Log:       log,
		Component: repository.AuditingComponent,
	}

	if timescaledbEnabled {
		backend, err := auditing.NewTimescaleDB(auditingCfg, auditing.TimescaleDbConfig{
			Host:      cli.String(auditingTimescaleHostFlag.Name),
			Port:      cli.String(auditingTimescalePortFlag.Name),
			DB:        cli.String(auditingTimescaleDbFlag.Name),
			User:      cli.String(auditingTimescaleUserFlag.Name),
			Password:  cli.String(auditingTimescalePasswordFlag.Name),
			Retention: cli.String(auditingTimescaleRetentionFlag.Name),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("unable to initialize timescaledb audit backend: %w", err)
		}

		backends = append(backends, backend)

		if cli.String(auditingSearchBackendFlag.Name) == auditingBackendTimescaleDB {
			log.Info("configured timescaledb audit backend as search backend")
			searchBackend = backend
		} else {
			log.Info("configured timescaledb audit backend")
		}
	}

	if splunkEnabled {
		host := cli.String(auditingSplunkHostFlag.Name)
		if host == "" {
			host, err = os.Hostname()
			if err != nil {
				return nil, nil, err
			}
		}

		source := cli.App.Name
		if s := cli.String(auditingSplunkSourceFlag.Name); s != "" {
			source = s
		}

		splunkConfig := auditing.SplunkConfig{
			Endpoint:   cli.String(auditingSplunkEndpointFlag.Name),
			HECToken:   cli.String(auditingSplunkHecTokenFlag.Name),
			SourceType: cli.String(auditingSplunkSourceTypeFlag.Name),
			Index:      cli.String(auditingSplunkIndexFlag.Name),
			Host:       host,
		}

		if caPath := cli.String(auditingSplunkCaFlag.Name); caPath != "" {
			caCert, err := os.ReadFile(caPath)
			if err != nil {
				return nil, nil, fmt.Errorf("unable to read ca cert: %w", err)
			}

			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			splunkConfig.TlsConfig = &tls.Config{
				RootCAs:    caCertPool,
				MinVersion: tls.VersionTLS12,
			}
		}

		splunkBackend, err := auditing.NewSplunk(auditing.Config{
			Component: source,
			Log:       log,
		}, splunkConfig)
		if err != nil {
			return nil, nil, err
		}

		asyncSplunkBackend, err := auditing.NewAsync(splunkBackend, log, auditing.AsyncConfig{
			AsyncRetry:   splunkAsyncRetry,
			AsyncBackoff: new(splunkAsyncBackoff),
		})
		if err != nil {
			return nil, nil, err
		}

		backends = append(backends, asyncSplunkBackend)

		if searchBackend == nil && cli.String(auditingSearchBackendFlag.Name) == auditingBackendSplunk {
			return nil, nil, fmt.Errorf("splunk is not supported as a search backend")
		} else {
			log.Info("configured splunk audit backend")
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
