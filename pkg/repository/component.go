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
)

const (
	componentPrefix = "component:"
)

type (
	ComponentEntity struct {
		*apiv2.Component
	}

	ComponentServiceCreateRequest struct {
		*apiv2.Component
		Expiration time.Duration
	}

	ComponentServiceUpdateRequest struct {
	}

	componentRepository struct {
		s *Store
	}
)

func (*ComponentServiceUpdateRequest) GetUpdateMeta() *apiv2.UpdateMeta {
	return &apiv2.UpdateMeta{}
}

func (e *ComponentEntity) SetChanged(time time.Time) {
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

func (c *componentRepository) get(ctx context.Context, id string) (*ComponentEntity, error) {
	keys, err := c.s.component.Do(ctx, c.s.component.B().Keys().Pattern(componentPrefix+"*").Build()).AsStrSlice()
	if err != nil {
		return nil, err
	}

	var result *ComponentEntity
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
			result = &ComponentEntity{Component: &component}
			break
		}
	}
	if result == nil {
		return nil, errorutil.NotFound("no component with uuid %s found", id)
	}
	return result, nil
}

func (c *componentRepository) validateCreate(ctx context.Context, rq *ComponentServiceCreateRequest) error {
	return nil
}

func (c *componentRepository) create(ctx context.Context, rq *ComponentServiceCreateRequest) (*ComponentEntity, error) {
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
	return &ComponentEntity{Component: rq.Component}, err
}

func (c *componentRepository) validateUpdate(ctx context.Context, rq *ComponentServiceUpdateRequest, old *ComponentEntity) error {
	panic("unimplemented")
}

func (c *componentRepository) update(ctx context.Context, e *ComponentEntity, msg *ComponentServiceUpdateRequest) (*ComponentEntity, error) {
	panic("unimplemented")
}

func (c *componentRepository) validateDelete(ctx context.Context, e *ComponentEntity) error {
	return nil
}

func (c *componentRepository) delete(ctx context.Context, e *ComponentEntity) error {
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

func (c *componentRepository) find(ctx context.Context, query *apiv2.ComponentQuery) (*ComponentEntity, error) {
	panic("unimplemented")
}

func (c *componentRepository) list(ctx context.Context, query *apiv2.ComponentQuery) ([]*ComponentEntity, error) {
	// TODO valkey-json would be a perfect fit: https://github.com/valkey-io/valkey-json
	keys, err := c.s.component.Do(ctx, c.s.component.B().Keys().Pattern(componentPrefix+"*").Build()).AsStrSlice()
	if err != nil {
		return nil, err
	}

	var result []*ComponentEntity
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

		e := &ComponentEntity{Component: &component}
		result = append(result, e)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Uuid > result[j].Uuid
	})

	return result, nil
}

func (c *componentRepository) convertToInternal(ctx context.Context, msg *apiv2.Component) (*ComponentEntity, error) {
	panic("unimplemented")
}

func (c *componentRepository) convertToProto(ctx context.Context, e *ComponentEntity) (*apiv2.Component, error) {
	return e.Component, nil
}

func (c *componentRepository) matchScope(e *ComponentEntity) bool {
	return true
}
