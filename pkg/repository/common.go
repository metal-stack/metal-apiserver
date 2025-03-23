package repository

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

func checkIfUrlExists(ctx context.Context, entity, id, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s:%s is not accessible under:%s error:%w", entity, id, url, err)
	}
	defer resp.Body.Close()

	// Consider 2xx and 3xx status codes as available
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
		return nil
	}

	return fmt.Errorf("%s:%s is not accessible under:%s statuscode:%d", entity, id, url, resp.StatusCode)
}
