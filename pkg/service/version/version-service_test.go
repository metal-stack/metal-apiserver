package version

import (
	"context"
	"log/slog"
	"reflect"
	"testing"

	apiv1 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/v"
)

func Test_versionServiceServer_Get(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		req      *apiv1.VersionServiceGetRequest
		log      *slog.Logger
		revision string
		version  string
		want     *apiv1.VersionServiceGetResponse
		wantErr  bool
	}{
		{
			name:     "simple",
			ctx:      t.Context(),
			req:      &apiv1.VersionServiceGetRequest{},
			revision: "abc",
			version:  "v0.0.1",
			log:      slog.Default(),
			want:     &apiv1.VersionServiceGetResponse{Version: &apiv1.Version{Version: "v0.0.1", Revision: "abc"}},
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			v.Revision = tt.revision
			v.Version = tt.version
			a := &versionServiceServer{
				log: tt.log,
			}
			if tt.wantErr == false {
				// Execute proto based validation
				test.Validate(t, tt.req)
			}
			got, err := a.Get(tt.ctx, tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("versionServiceServer.Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("versionServiceServer.Get() = %v, want %v", got, tt.want)
			}
		})
	}
}
