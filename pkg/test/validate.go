package test

import (
	"testing"

	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func Validate(t *testing.T, msg proto.Message, options ...protovalidate.ValidationOption) {
	err := protovalidate.Validate(msg, options...)
	require.NoError(t, err)
}
