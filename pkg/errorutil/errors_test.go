package errorutil

import (
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
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
			err:  NotFound("ip was not found"),
			want: connect.NewError(connect.CodeNotFound, errors.New("ip was not found")),
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

func TestNotFound(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		args    []interface{}
		wantErr bool
	}{
		{
			name:    "TestNotFound 1",
			format:  "SomeFormat",
			wantErr: true,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			if err := NotFound(tt.format, tt.args...); (err != nil) != tt.wantErr {
				t.Errorf("NotFound() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func _TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "Test 1",
			err:  errors.New("Some other Error"),
			want: false,
		},
		{
			name: "Test 2",
			err:  connect.NewError(connect.CodeNotFound, errors.New("")),
			want: true,
		},
		{
			name: "Test 3",
			err:  nil,
			want: false,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsConflict(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "Test 1",
			err:  errors.New("Some other Error"),
			want: false,
		},
		{
			name: "Test 2",
			err:  connect.NewError(connect.CodeAlreadyExists, errors.New("")),
			want: true,
		},
		{
			name: "Test 3",
			err:  nil,
			want: false,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			if got := IsConflict(tt.err); got != tt.want {
				t.Errorf("IsConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsInternal(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "Test 1",
			err:  errors.New("Some other Error"),
			want: false,
		},
		{
			name: "Test 2",
			err:  connect.NewError(connect.CodeInternal, errors.New("")),
			want: true,
		},
		{
			name: "Test 3",
			err:  nil,
			want: false,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			if got := IsInternal(tt.err); got != tt.want {
				t.Errorf("IsInternal() = %v, want %v", got, tt.want)
			}
		})
	}
}
