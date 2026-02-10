package test_test

import (
	"testing"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/test"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/stretchr/testify/require"
)

func Test_partitionUpdateWithDatacenter(t *testing.T) {
	// FIXME enable once working
	t.Skip()
	t.Parallel()

	dc := test.NewDatacenter(t, &test.DatacenterConfig{Partitions: pointer.Pointer(uint(2))})
	defer dc.Close()

	dc.Assert(&test.Asserters{
		Partition: func(t testing.TB, partition *apiv2.Partition) {
			require.Equal(t, "bla", partition.Description)
		},
	})

}
