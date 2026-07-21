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
	auditingsplunk "github.com/metal-stack/metal-lib/auditing/splunk"
	auditingtimescaledb "github.com/metal-stack/metal-lib/auditing/timescaledb"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/urfave/cli/v3"
)

// might return (nil, nil, nil) if auditing is disabled!
func createAuditingClient(cmd *cli.Command, log *slog.Logger) (searchBackend auditing.Auditing, backends []auditing.Auditing, err error) {
	const (
		auditingBackendTimescaleDB = "timescaledb"
		auditingBackendSplunk      = "splunk"
	)

	var (
		auditingCfg = auditing.Config{
			Log:       log,
			Component: api.AuditingComponent,
		}

		timescaledbEnabled = cmd.Bool(auditingTimescaleEnabledFlag.Name)
		splunkEnabled      = cmd.Bool(auditingSplunkEnabledFlag.Name)

		auditingEnabled = timescaledbEnabled || splunkEnabled
	)

	if !auditingEnabled {
		return nil, nil, nil
	}

	if timescaledbEnabled {
		backend, err := newTimescaledbBackend(cmd, auditingCfg)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to initialize timescaledb audit backend: %w", err)
		}

		backends = append(backends, backend)

		log.Info("configured timescaledb audit backend")

		if cmd.String(auditingSearchBackendFlag.Name) == auditingBackendTimescaleDB {
			log.Info("using timescaledb audit backend as search backend")
			searchBackend = backend
		}
	}

	if splunkEnabled {
		backend, err := newSplunkBackend(cmd, auditingCfg)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to initialize splunk audit backend: %w", err)
		}

		backends = append(backends, backend)

		log.Info("configured splunk audit backend")

		if searchBackend == nil && cmd.String(auditingSearchBackendFlag.Name) == auditingBackendSplunk {
			return nil, nil, fmt.Errorf("splunk is not supported as a search backend")
		}
	}

	if backendName := cmd.String(auditingSearchBackendFlag.Name); backendName == "" {
		searchBackend = pointer.FirstOrZero(backends)
		log.Info("defaulted audit search backend", "backend-type", fmt.Sprintf("%T", searchBackend))
	} else if searchBackend == nil {
		return nil, nil, fmt.Errorf("search backend not supported or unconfigured: %s", backendName)
	}

	return
}

func newTimescaledbBackend(cmd *cli.Command, auditingCfg auditing.Config) (searchBackend auditing.Auditing, err error) {
	return auditingtimescaledb.NewTimescaleDB(auditingCfg, auditingtimescaledb.TimescaleDbConfig{
		Host:      cmd.String(auditingTimescaleHostFlag.Name),
		Port:      cmd.String(auditingTimescalePortFlag.Name),
		DB:        cmd.String(auditingTimescaleDbFlag.Name),
		User:      cmd.String(auditingTimescaleUserFlag.Name),
		Password:  cmd.String(auditingTimescalePasswordFlag.Name),
		Retention: cmd.String(auditingTimescaleRetentionFlag.Name),
	})
}

func newSplunkBackend(cmd *cli.Command, auditingCfg auditing.Config) (searchBackend auditing.Auditing, err error) {
	const (
		splunkAsyncBackoff = 1 * time.Second
		splunkAsyncRetry   = 3
	)

	host := cmd.String(auditingSplunkHostFlag.Name)
	if host == "" {
		host, err = os.Hostname()
		if err != nil {
			return nil, err
		}
	}

	source := cmd.Name
	if s := cmd.String(auditingSplunkSourceFlag.Name); s != "" {
		source = s
	}

	splunkConfig := auditingsplunk.SplunkConfig{
		Endpoint:   cmd.String(auditingSplunkEndpointFlag.Name),
		HECToken:   cmd.String(auditingSplunkHecTokenFlag.Name),
		SourceType: cmd.String(auditingSplunkSourceTypeFlag.Name),
		Index:      cmd.String(auditingSplunkIndexFlag.Name),
		Host:       host,
	}

	if caPath := cmd.String(auditingSplunkCaFlag.Name); caPath != "" {
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

	splunkBackend, err := auditingsplunk.NewSplunk(auditing.Config{
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
