package headscale

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func (h Client) Version(ctx context.Context) (string, error) {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.controllerURL+"/version", nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("performing request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	version, ok := result["version"]
	if !ok {
		return "", fmt.Errorf("no version field in response %v", result)
	}

	versionString, ok := version.(string)
	if !ok {
		return "", fmt.Errorf("error converting versionresponse to string")
	}

	return versionString, nil
}
