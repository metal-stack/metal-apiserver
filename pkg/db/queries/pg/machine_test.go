package pg_test

import (
	"testing"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/generic/pg"
	querypg "github.com/metal-stack/metal-apiserver/pkg/db/queries/pg"
	"github.com/stretchr/testify/require"
)

func TestMachineFilter(t *testing.T) {
	partition := "partition-1"
	size := "s1.xlarge"
	rack := "rack-1"
	room := "room-1"
	name := "machine-1"
	waiting := true
	preallocated := false
	project := "project-1"
	state := apiv2.MachineState_MACHINE_STATE_TAINTED

	filters := querypg.MachineFilter(&apiv2.MachineQuery{
		Partition:    &partition,
		Size:         &size,
		Rack:         &rack,
		Room:         &room,
		Name:         &name,
		Waiting:      &waiting,
		Preallocated: &preallocated,
		State:        &state,
		Allocation: &apiv2.MachineAllocationQuery{
			Project: &project,
		},
	})

	require.Equal(t, []string{
		"Name",
		"PartitionID",
		"SizeID",
		"RackID",
		"RoomID",
		"Waiting",
		"PreAllocated",
		"State.Value",
		"Allocation.Project",
	}, pathsOf(filters))

	byPath := map[string]any{}
	for _, f := range filters {
		byPath[f.Path] = f.Value
	}

	require.Equal(t, "partition-1", byPath["PartitionID"])
	require.Equal(t, "s1.xlarge", byPath["SizeID"])
	require.Equal(t, "rack-1", byPath["RackID"])
	require.Equal(t, "room-1", byPath["RoomID"])
	require.Equal(t, "machine-1", byPath["Name"])
	require.Equal(t, "project-1", byPath["Allocation.Project"])
	require.Equal(t, true, byPath["Waiting"])
	require.Equal(t, false, byPath["PreAllocated"])
	require.Equal(t, "TAINTED", byPath["State.Value"])
}

func TestMachineFilterNilQuery(t *testing.T) {
	require.Nil(t, querypg.MachineFilter(nil))
}

func TestMachineFilterAllocationType(t *testing.T) {
	allocType := apiv2.MachineAllocationType_MACHINE_ALLOCATION_TYPE_FIREWALL

	filters := querypg.MachineFilter(&apiv2.MachineQuery{
		Allocation: &apiv2.MachineAllocationQuery{
			AllocationType: &allocType,
		},
	})

	require.Len(t, filters, 1)
	require.Equal(t, "Allocation.Role", filters[0].Path)
	require.Equal(t, "firewall", filters[0].Value)
}

func pathsOf(filters []pg.QueryFilter) []string {
	var out []string
	for _, f := range filters {
		out = append(out, f.Path)
	}
	return out
}
