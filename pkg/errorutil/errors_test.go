package errorutil

import (
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want *connect.Error
	}{
		{
			name: "generic",
			err:  generic.NotFound("ip was not found"),
			want: connect.NewError(connect.CodeNotFound, errors.New("NotFound ip was not found")),
		},
		{
			name: "masterdata",
			err:  status.Error(codes.NotFound, "tenant not found"),
			want: connect.NewError(connect.CodeNotFound, errors.New("tenant not found")),
		},
		{
			name: "ipam",
			err:  connect.NewError(connect.CodeNotFound, fmt.Errorf("prefix not found")),
			want: connect.NewError(connect.CodeNotFound, errors.New("prefix not found")),
		},
		{
			name: "ipam",
			err:  connect.NewError(connect.CodeCanceled, fmt.Errorf("canceled")),
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := notFound(tt.err)
			if tt.want != nil {
				require.EqualError(t, got, tt.want.Error())
			} else {
				require.Nil(t, got)
			}
		})
	}
}
