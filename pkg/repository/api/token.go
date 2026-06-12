package api

import (
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

type TokenWithSecret struct {
	Token *apiv2.Token
	// Secret is only filled after creation or refresh, otherwise this is an empty string
	Secret string
}

func (t *TokenWithSecret) SetChanged(time time.Time) {}

func (t *TokenWithSecret) GetMeta() *apiv2.Meta {
	return t.Token.Meta
}
