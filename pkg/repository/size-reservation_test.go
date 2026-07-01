package repository

import (
	"slices"
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/stretchr/testify/require"
)

func Test_checkSizeReservations(t *testing.T) {
	var (
		available = []*metal.Machine{
			{Base: metal.Base{ID: "1"}},
			{Base: metal.Base{ID: "2"}},
			{Base: metal.Base{ID: "3"}},
			{Base: metal.Base{ID: "4"}},
			{Base: metal.Base{ID: "5"}},
		}

		partitionA = "a"
		p0         = "0"
		p1         = "1"
		p2         = "2"

		reservations = []*metal.SizeReservation{
			{
				SizeID:       "c1-xlarge-x86",
				Amount:       1,
				ProjectID:    p1,
				PartitionIDs: []string{partitionA},
			},
			{
				SizeID:       "c1-xlarge-x86",
				Amount:       2,
				ProjectID:    p2,
				PartitionIDs: []string{partitionA},
			},
		}

		projectMachines = map[string][]*metal.Machine{}

		allocate = func(id, project string) {
			available = slices.DeleteFunc(available, func(m *metal.Machine) bool {
				return m.ID == id
			})
			projectMachines[project] = append(projectMachines[project], &metal.Machine{Base: metal.Base{ID: id}})
		}
	)

	// 5 available, 3 reserved, project 0 can allocate
	r := &sizeReservationRepository{}
	ok := r.checkSizeReservations(available, p0, projectMachines, reservations)
	require.True(t, ok)
	allocate(available[0].ID, p0)

	require.Equal(t, []*metal.Machine{
		{Base: metal.Base{ID: "2"}},
		{Base: metal.Base{ID: "3"}},
		{Base: metal.Base{ID: "4"}},
		{Base: metal.Base{ID: "5"}},
	}, available)
	require.Equal(t, map[string][]*metal.Machine{
		p0: {
			{Base: metal.Base{ID: "1"}},
		},
	}, projectMachines)

	// 4 available, 3 reserved, project 2 can allocate
	ok = r.checkSizeReservations(available, p2, projectMachines, reservations)
	require.True(t, ok)
	allocate(available[0].ID, p2)

	require.Equal(t, []*metal.Machine{
		{Base: metal.Base{ID: "3"}},
		{Base: metal.Base{ID: "4"}},
		{Base: metal.Base{ID: "5"}},
	}, available)
	require.Equal(t, map[string][]*metal.Machine{
		p0: {
			{Base: metal.Base{ID: "1"}},
		},
		p2: {
			{Base: metal.Base{ID: "2"}},
		},
	}, projectMachines)

	// 3 available, 3 reserved (1 used), project 0 can allocate
	ok = r.checkSizeReservations(available, p0, projectMachines, reservations)
	require.True(t, ok)
	allocate(available[0].ID, p0)

	require.Equal(t, []*metal.Machine{
		{Base: metal.Base{ID: "4"}},
		{Base: metal.Base{ID: "5"}},
	}, available)
	require.Equal(t, map[string][]*metal.Machine{
		p0: {
			{Base: metal.Base{ID: "1"}},
			{Base: metal.Base{ID: "3"}},
		},
		p2: {
			{Base: metal.Base{ID: "2"}},
		},
	}, projectMachines)

	// 2 available, 3 reserved (1 used), project 0 cannot allocate anymore
	ok = r.checkSizeReservations(available, p0, projectMachines, reservations)
	require.False(t, ok)

	// 2 available, 3 reserved (1 used), project 2 can allocate
	ok = r.checkSizeReservations(available, p2, projectMachines, reservations)
	require.True(t, ok)
	allocate(available[0].ID, p2)

	require.Equal(t, []*metal.Machine{
		{Base: metal.Base{ID: "5"}},
	}, available)
	require.Equal(t, map[string][]*metal.Machine{
		p0: {
			{Base: metal.Base{ID: "1"}},
			{Base: metal.Base{ID: "3"}},
		},
		p2: {
			{Base: metal.Base{ID: "2"}},
			{Base: metal.Base{ID: "4"}},
		},
	}, projectMachines)

	// 1 available, 3 reserved (2 used), project 0 and 2 cannot allocate anymore
	ok = r.checkSizeReservations(available, p0, projectMachines, reservations)
	require.False(t, ok)
	ok = r.checkSizeReservations(available, p2, projectMachines, reservations)
	require.False(t, ok)

	// 1 available, 3 reserved (2 used), project 1 can allocate
	ok = r.checkSizeReservations(available, p1, projectMachines, reservations)
	require.True(t, ok)
	allocate(available[0].ID, p1)

	require.Equal(t, []*metal.Machine{}, available)
	require.Equal(t, map[string][]*metal.Machine{
		p0: {
			{Base: metal.Base{ID: "1"}},
			{Base: metal.Base{ID: "3"}},
		},
		p1: {
			{Base: metal.Base{ID: "5"}},
		},
		p2: {
			{Base: metal.Base{ID: "2"}},
			{Base: metal.Base{ID: "4"}},
		},
	}, projectMachines)
}
