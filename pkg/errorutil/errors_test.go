package errorutil

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/metal-stack/api-server/pkg/db/generic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "generic",
			err:  generic.NotFound("ip was not found"),
			want: true,
		},
		{
			name: "masterdata",
			err:  status.Error(codes.NotFound, "tenant not found"),
			want: true,
		},
		{
			name: "ipam",
			err:  connect.NewError(connect.CodeNotFound, fmt.Errorf("prefix not found")),
			want: true,
		},
		{
			name: "ipam",
			err:  connect.NewError(connect.CodeCanceled, fmt.Errorf("canceled")),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}
