package repository

import (
	"errors"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

func (r *machineRepository) selectMachine(allMachines, projectMachines []*metal.Machine, tags []string) (*metal.Machine, error) {
	spreadCandidates := r.spreadAcrossRacks(allMachines, projectMachines, tags)
	if len(spreadCandidates) == 0 {
		return nil, errors.New("no machine available")
	}

	machine := spreadCandidates[randomIndex(len(spreadCandidates))]
	return machine, nil
}

func (r *machineRepository) spreadAcrossRacks(allMachines, projectMachines []*metal.Machine, tags []string) []*metal.Machine {
	var (
		allRacks = groupByRack(allMachines)

		projectRacks                = groupByRack(projectMachines)
		leastOccupiedByProjectRacks = electRacks(allRacks, projectRacks)

		taggedMachines           = groupByTags(projectMachines).filter(tags...).getMachines()
		taggedRacks              = groupByRack(taggedMachines)
		leastOccupiedByTagsRacks = electRacks(allRacks, taggedRacks)

		intersection = intersect(leastOccupiedByTagsRacks, leastOccupiedByProjectRacks)
	)

	if c := allRacks.filter(intersection...).getMachines(); len(c) > 0 {
		return c
	}

	return allRacks.filter(leastOccupiedByTagsRacks...).getMachines() // tags have precedence over project
}

type groupedMachines map[string][]*metal.Machine

func groupByRack(machines []*metal.Machine) groupedMachines {
	racks := make(groupedMachines)

	for _, m := range machines {
		racks[m.RackID] = append(racks[m.RackID], m)
	}

	return racks
}

func (g groupedMachines) getMachines() []*metal.Machine {
	machines := make([]*metal.Machine, 0)

	for id := range g {
		machines = append(machines, g[id]...)
	}

	return machines
}

func (g groupedMachines) filter(keys ...string) groupedMachines {
	result := make(groupedMachines)

	for i := range keys {
		ms, ok := g[keys[i]]
		if ok {
			result[keys[i]] = ms
		}
	}

	return result
}

// electRacks returns the least occupied racks from all racks
func electRacks(allRacks, occupiedRacks groupedMachines) []string {
	winners := make([]string, 0)
	min := math.MaxInt

	for id := range allRacks {
		if _, ok := occupiedRacks[id]; ok {
			continue
		}
		occupiedRacks[id] = nil
	}

	for id := range occupiedRacks {
		if _, ok := allRacks[id]; !ok {
			continue
		}

		switch {
		case len(occupiedRacks[id]) < min:
			min = len(occupiedRacks[id])
			winners = []string{id}
		case len(occupiedRacks[id]) == min:
			winners = append(winners, id)
		}
	}

	return winners
}

func groupByTags(machines []*metal.Machine) groupedMachines {
	groups := make(groupedMachines)

	for _, m := range machines {
		for j := range m.Tags {
			ms := groups[m.Tags[j]]
			groups[m.Tags[j]] = append(ms, m)
		}
	}

	return groups
}

func randomIndex(max int) int {
	if max <= 0 {
		return 0
	}
	// golangci-lint has an issue with math/rand/v2
	// here it provides sufficient randomness though because it's not used for cryptographic purposes
	return rand.N(max) //nolint:gosec
}

func intersect[T comparable](a, b []T) []T {
	c := make([]T, 0)

	for i := range a {
		if slices.Contains(b, a[i]) {
			c = append(c, a[i])
		}
	}

	return c
}
