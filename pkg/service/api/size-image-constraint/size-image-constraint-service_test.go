package sizeimageconstraint

import (
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	sc "github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
)

func Test_sizeImageConstraintServiceServer_Try(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	dc := test.NewDatacenter(t, log)
	dc.Create(&sc.DatacenterSpec{
		Sizes:  sc.DefaultDatacenter.Sizes,
		Images: sc.DefaultDatacenter.Images,
		SizeImageConstraints: []*adminv2.SizeImageConstraintServiceCreateRequest{
			{
				Size: sc.SizeC1Large,
				ImageConstraints: []*apiv2.ImageConstraint{
					{
						Image:       "debian",
						SemverMatch: ">= 13.0",
					},
				},
			},
		},
	})

	tests := []struct {
		name    string
		req     *apiv2.SizeImageConstraintServiceTryRequest
		want    *apiv2.SizeImageConstraintServiceTryResponse
		wantErr error
	}{
		{
			name:    "debian 12 does not match",
			req:     &apiv2.SizeImageConstraintServiceTryRequest{Size: sc.SizeC1Large, Image: "debian-12.0.20251220"},
			want:    nil,
			wantErr: errorutil.InvalidArgument("given size:c1-large-x86 with image:debian-12.0.20251220 does violate image constraint:debian >=13.0"),
		},
		{
			name:    "debian 13 match",
			req:     &apiv2.SizeImageConstraintServiceTryRequest{Size: sc.SizeC1Large, Image: "debian-13.0.20260131"},
			want:    nil,
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &sizeImageConstraintServiceServer{
				log:  log,
				repo: dc.GetTestStore().Store,
			}
			_, err := s.Try(t.Context(), tt.req)
			if diff := cmp.Diff(err, tt.wantErr, errorutil.ConnectErrorComparer()); diff != "" {
				t.Errorf("diff = %s", diff)
			}
		})
	}
}
