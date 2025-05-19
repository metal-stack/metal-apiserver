package errorutil

import (
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/metal-lib/pkg/testcommon"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// func TestNotFoundInternal(t *testing.T) {
// 	tests := []struct {
// 		name string
// 		err  error
// 		want *connect.Error
// 	}{
// 		{
// 			name: "generic",
// 			err:  NotFound("ip was not found"),
// 			want: connect.NewError(connect.CodeNotFound, errors.New("ip was not found")),
// 		},
// 		{
// 			name: "masterdata",
// 			err:  status.Error(codes.NotFound, "tenant not found"),
// 			want: connect.NewError(connect.CodeNotFound, errors.New("tenant not found")),
// 		},
// 		{
// 			name: "ipam",
// 			err:  connect.NewError(connect.CodeNotFound, fmt.Errorf("prefix not found")),
// 			want: connect.NewError(connect.CodeNotFound, errors.New("prefix not found")),
// 		},
// 		{
// 			name: "ipam",
// 			err:  connect.NewError(connect.CodeCanceled, fmt.Errorf("canceled")),
// 			want: nil,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			got := notFound(tt.err)
// 			if tt.want != nil {
// 				require.EqualError(t, got, tt.want.Error())
// 			} else {
// 				require.Nil(t, got)
// 			}
// 		})
// 	}
// }

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

func TestIsNotFound(t *testing.T) {
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
			want: true,
		},
		{
			name: "Test 2",
			err:  connect.NewError(connect.CodeInternal, errors.New("")),
			want: true,
		},
		{
			name: "Test 2",
			err:  connect.NewError(connect.CodeAborted, errors.New("")),
			want: false,
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

func TestWrappedInternal(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr error
	}{
		{
			name:    "one error",
			err:     Internal("something went wrong"),
			wantErr: connect.NewError(connect.CodeInternal, errors.New("something went wrong")),
		},
		{
			name:    "two errors",
			err:     Internal("wrapping error: %w", fmt.Errorf("root error")),
			wantErr: connect.NewError(connect.CodeInternal, fmt.Errorf("wrapping error: %w", errors.New("root error"))),
		},
		{
			name:    "two errors upside down",
			err:     fmt.Errorf("wrapping error: %w", Internal("root error")),
			wantErr: fmt.Errorf("wrapping error: %w", connect.NewError(connect.CodeInternal, errors.New("root error"))),
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.wantErr, tt.err, testcommon.ErrorStringComparer()); diff != "" {
				t.Errorf("wrapping() %v", diff)
			}
		})
	}
}

func TestConvert(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr error
	}{
		{
			name:    "non specific error",
			err:     fmt.Errorf("something went wrong"),
			wantErr: connect.NewError(connect.CodeInternal, errors.New("something went wrong")),
		},
		{
			name:    "passing connect error",
			err:     Internal("something went wrong"),
			wantErr: connect.NewError(connect.CodeInternal, errors.New("something went wrong")),
		},
		{
			name:    "wrapped connect error",
			err:     Internal("wrapping error: %w", fmt.Errorf("root error")),
			wantErr: connect.NewError(connect.CodeInternal, fmt.Errorf("wrapping error: %w", errors.New("root error"))),
		},
		{
			name:    "wrapped connect error gets unwrapped",
			err:     fmt.Errorf("wrapping error: %w", Internal("root error")),
			wantErr: connect.NewError(connect.CodeInternal, fmt.Errorf("wrapping error: root error")),
		},
		{
			name:    "grpc error gets converted",
			err:     status.Errorf(codes.AlreadyExists, "project already exists"), // TODO: check if this is really returned by a grpc client like that
			wantErr: connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("project already exists")),
		},
		{
			name:    "wrapped grpc error gets unwrapped",
			err:     fmt.Errorf("wrapping error: %w", status.Errorf(codes.AlreadyExists, "project already exists")),
			wantErr: connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("wrapping error: project already exists")),
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.wantErr, Convert(tt.err), testcommon.ErrorStringComparer()); diff != "" {
				t.Errorf("wrapping() %v", diff)
			}
		})
	}
}
