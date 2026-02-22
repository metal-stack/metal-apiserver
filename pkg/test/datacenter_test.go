package test_test

import (
	"log/slog"
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
)

func Test_partitionUpdateWithDatacenter(t *testing.T) {
	t.Parallel()

	dc := test.NewDatacenter(t, slog.Default())
	dc.Create(&scenarios.DefaultDatacenter)
	defer dc.Close()

	dc.Dump()
	dc.CleanUp()
}
