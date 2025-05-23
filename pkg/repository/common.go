package repository

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
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
	defer func() {
		_ = resp.Body.Close()
	}()
	// Consider 2xx and 3xx status codes as available
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
		return nil
	}

	return fmt.Errorf("%s:%s is not accessible under:%s statuscode:%d", entity, id, url, resp.StatusCode)
}

// validate condition, if condition evaluates to false, append formatted error message to given slice of errors.
// a slice of errors must be passed
func validate(errs []error, condition bool, format string, args ...any) []error {
	if !condition {
		return append(errs, fmt.Errorf(format, args...))
	}
	return errs
}

func updateLabelsOnSlice(rq *apiv2.UpdateLabels, existingTags []string) []string {
	// we cannot easily use tag.TagMap here because it would not maintain labels that have no assignment
	// to prevent a breaking change, we implemented own logic here
	// as soon as a user touches a pure label though, it will be transformed into the map format because
	// the current api definition does not allow pure labels

	var (
		pureLabels []string
		tagMap     = map[string]string{}
	)

	for _, l := range existingTags {
		key, value, ok := strings.Cut(l, "=")
		if ok {
			tagMap[key] = value
		} else {
			pureLabels = append(pureLabels, l)
		}
	}

	for _, remove := range rq.Remove {
		// TODO: do we want to return an error in case the label did not exist before?
		delete(tagMap, remove)
		pureLabels = slices.DeleteFunc(pureLabels, func(l string) bool { return l == remove })
	}

	if rq.Update != nil {
		for k, v := range rq.Update.Labels {
			if slices.Contains(pureLabels, k) {
				pureLabels = slices.DeleteFunc(pureLabels, func(l string) bool { return l == k })
			}

			tagMap[k] = v
		}
	}

	var newTags []string
	for k, v := range tagMap {
		newTags = append(newTags, fmt.Sprintf("%s=%s", k, v))
	}
	newTags = append(newTags, pureLabels...)

	slices.Sort(newTags)

	return newTags
}

func updateLabelsOnMap(rq *apiv2.UpdateLabels, existingLabels map[string]string) map[string]string {
	var result map[string]string

	if existingLabels != nil {
		result = make(map[string]string)
		maps.Copy(result, existingLabels)
	}

	for _, remove := range rq.Remove {
		delete(result, remove)
	}

	if rq.Update != nil {
		for k, v := range rq.Update.Labels {
			if result == nil {
				result = make(map[string]string)
			}
			result[k] = v
		}
	}

	return result
}

func checkAlreadyExists[E generic.Entity](ctx context.Context, s generic.Storage[E], id string) bool {
	_, err := s.Get(ctx, id)
	return !errorutil.IsNotFound(err)
}
