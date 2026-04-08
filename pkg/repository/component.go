package repository

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/repository/api"
)

const (
	componentPrefix = "component:"
)

type (
	componentEntity struct {
		*apiv2.Component
	}

	componentRepository struct {
		s *Store
	}
)

func (e *componentEntity) SetChanged(time time.Time) {
}

func id(typeString, identifier string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(typeString+":"+identifier)).String()
}

func (c *componentRepository) key(rq *apiv2.Component) (string, error) {
	typeAsString, err := enum.GetStringValue(rq.Type)
	if err != nil {
		return "", err
	}

	key := componentPrefix + ":" + *typeAsString + ":" + id(*typeAsString, rq.Identifier)
	return key, nil
}

func (c *componentRepository) get(ctx context.Context, id string) (*componentEntity, error) {
	keys, err := c.s.component.Do(ctx, c.s.component.B().Keys().Pattern(componentPrefix+"*").Build()).AsStrSlice()
	if err != nil {
		return nil, err
	}

	var result *componentEntity
	for _, key := range keys {
		value, err := c.s.component.Do(ctx, c.s.component.B().Get().Key(key).Build()).AsBytes()
		if err != nil {
			return nil, err
		}

		var component apiv2.Component
		err = json.Unmarshal(value, &component)
		if err != nil {
			return nil, err
		}

		if component.Uuid == id {
			result = &componentEntity{Component: &component}
			break
		}
	}
	if result == nil {
		return nil, errorutil.NotFound("no component with uuid %s found", id)
	}
	return result, nil
}

func (c *componentRepository) validateCreate(ctx context.Context, rq *api.ComponentServiceCreateRequest) error {
	return nil
}

func (c *componentRepository) create(ctx context.Context, rq *api.ComponentServiceCreateRequest) (*componentEntity, error) {
	payload, err := json.Marshal(rq)
	if err != nil {
		return nil, err
	}

	key, err := c.key(rq.Component)
	if err != nil {
		return nil, err
	}

	err = c.s.component.Do(ctx, c.s.component.B().Set().Key(key).Value(string(payload)).Ex(rq.Expiration).Build()).Error()
	if err != nil {
		return nil, err
	}
	return &componentEntity{Component: rq.Component}, err
}

func (c *componentRepository) validateUpdate(ctx context.Context, rq *api.ComponentServiceUpdateRequest, old *componentEntity) error {
	panic("unimplemented")
}

func (c *componentRepository) update(ctx context.Context, e *componentEntity, msg *api.ComponentServiceUpdateRequest) (*componentEntity, error) {
	panic("unimplemented")
}

func (c *componentRepository) validateDelete(ctx context.Context, e *componentEntity) error {
	return nil
}

func (c *componentRepository) delete(ctx context.Context, e *componentEntity) error {
	resp, err := c.get(ctx, e.Uuid)
	if err != nil {
		return err
	}

	key, err := c.key(resp.Component)
	if err != nil {
		return err
	}

	return c.s.component.Do(ctx, c.s.component.B().Del().Key(key).Build()).Error()
}

func (c *componentRepository) find(ctx context.Context, query *apiv2.ComponentQuery) (*componentEntity, error) {
	panic("unimplemented")
}

func (c *componentRepository) list(ctx context.Context, query *apiv2.ComponentQuery) ([]*componentEntity, error) {
	// TODO valkey-json would be a perfect fit: https://github.com/valkey-io/valkey-json
	keys, err := c.s.component.Do(ctx, c.s.component.B().Keys().Pattern(componentPrefix+"*").Build()).AsStrSlice()
	if err != nil {
		return nil, err
	}

	var result []*componentEntity
	for _, key := range keys {
		value, err := c.s.component.Do(ctx, c.s.component.B().Get().Key(key).Build()).AsBytes()
		if err != nil {
			return nil, err
		}

		var component apiv2.Component
		err = json.Unmarshal(value, &component)
		if err != nil {
			return nil, err
		}

		if query != nil {
			if query.Uuid != nil && component.Uuid != *query.Uuid {
				continue
			}
			if query.Identifier != nil && component.Identifier != *query.Identifier {
				continue
			}
			if query.Type != nil && component.Type != *query.Type {
				continue
			}
		}

		e := &componentEntity{Component: &component}
		result = append(result, e)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Uuid > result[j].Uuid
	})

	return result, nil
}

func (c *componentRepository) convertToInternal(ctx context.Context, msg *apiv2.Component) (*componentEntity, error) {
	panic("unimplemented")
}

func (c *componentRepository) convertToProto(ctx context.Context, e *componentEntity) (*apiv2.Component, error) {
	return e.Component, nil
}

func (c *componentRepository) matchScope(e *componentEntity) bool {
	return true
}
