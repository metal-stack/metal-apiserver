// secret.go
// Stores a key/value pair in a Kubernetes Secret using only the Go standard library.
// It reads the in-cluster service-account token and CA cert, then calls the K8s API directly.
package k8s

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
)

const (
	inClusterTokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	inClusterCAFile    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	inClusterHost      = "https://kubernetes.default.svc"
)

// Minimal Secret structure – only the fields we need.
type Secret struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   map[string]string `json:"metadata"`
	Data       map[string]string `json:"data"` // base64-encoded values
}

func CreateOrUpdateSecret(ctx context.Context, log *slog.Logger, namespace, secretName, key, value string) error {

	// ── 1. Build HTTP client ────────────────────────────────────────────────
	var (
		client *http.Client
		token  string
		host   = inClusterHost
		url    = fmt.Sprintf("%s/api/v1/namespaces/%s/secrets/%s", host, namespace, secretName)
	)

	// In-cluster: use service-account token + CA bundle

	caData, err := os.ReadFile(inClusterCAFile)
	if err != nil {
		return fmt.Errorf("read CA: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caData)

	client = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}

	tokenBytes, err := os.ReadFile(inClusterTokenFile)
	if err != nil {
		return fmt.Errorf("read token: %v", err)
	}
	token = string(tokenBytes)

	// ── 2. Try GET to decide between CREATE and UPDATE ──────────────────────
	exists, currentData, err := getSecret(client, url, token)
	if err != nil {
		return fmt.Errorf("GET secret: %v", err)
	}

	// Merge: keep existing keys, overwrite/add our key.
	if currentData == nil {
		currentData = map[string]string{}
	}
	currentData[key] = base64.StdEncoding.EncodeToString([]byte(value))

	secret := Secret{
		APIVersion: "v1",
		Kind:       "Secret",
		Metadata: map[string]string{
			"name":      secretName,
			"namespace": namespace,
		},
		Data: currentData,
	}

	body, err := json.Marshal(secret)
	if err != nil {
		return fmt.Errorf("marshal: %v", err)
	}

	// ── 3. CREATE (POST) or UPDATE (PUT) ────────────────────────────────────
	var (
		method string
		reqURL string
	)

	if exists {
		method = http.MethodPut // full replace
		reqURL = url
	} else {
		method = http.MethodPost // create
		reqURL = fmt.Sprintf("%s/api/v1/namespaces/%s/secrets", host, namespace)
	}

	status, respBody, err := doRequest(client, method, reqURL, token, body)
	if err != nil {
		return fmt.Errorf("request: %v", err)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("unexpected status %d: %s", status, respBody)
	}

	log.Info("secret in namespace updated", "secret", secretName, "namespace", namespace, "key", key, "value", value, "http status", status)

	return nil
}

// getSecret GETs the secret and returns (exists, current base64 data, error).
func getSecret(client *http.Client, url, token string) (bool, map[string]string, error) {
	status, body, err := doRequest(client, http.MethodGet, url, token, nil)
	if err != nil {
		return false, nil, err
	}
	if status == http.StatusNotFound {
		return false, nil, nil
	}
	if status < 200 || status >= 300 {
		return false, nil, fmt.Errorf("GET status %d: %s", status, body)
	}

	var s Secret
	if err := json.Unmarshal(body, &s); err != nil {
		return false, nil, fmt.Errorf("unmarshal: %w", err)
	}
	return true, s.Data, nil
}

// doRequest is a thin wrapper around http.Request.
func doRequest(client *http.Client, method, url, token string, body []byte) (int, []byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, err
}
