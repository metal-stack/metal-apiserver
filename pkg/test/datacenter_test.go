package test_test

import (
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-apiserver/pkg/test/scenarios"
)

func Test_partitionUpdateWithDatacenter(t *testing.T) {
	t.Parallel()

	dc := test.NewDatacenter(t)
	dc.Create(&scenarios.DefaultDatacenter)
	defer dc.Close()

	// TODO make these asserters useful
	// dc.Assert(&test.Asserters{
	// 	Partition: func(t testing.TB, partition *apiv2.Partition) {
	// 		assert.Equal(t, "bla", partition.Description)
	// 	},
	// })

	dc.Dump(t)
	dc.CleanUp(t)
}
