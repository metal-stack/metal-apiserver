package scenarios

import (
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

type (
	MachineWithLiveliness[T, U any] struct {
		Liveliness T
		Machine    U
	}

	DatacenterSpec struct {
		Partitions        []string
		Tenants           []string
		ProjectsPerTenant int
		Images            map[string]apiv2.ImageFeature
		Sizes             []*apiv2.Size
		SizeReservations  []*adminv2.SizeReservationServiceCreateRequest
		Networks          []*adminv2.NetworkServiceCreateRequest
		IPs               []*apiv2.IPServiceCreateRequest
		Switches          []*apiv2.Switch
		Machines          []*MachineWithLiveliness[metal.MachineLiveliness, *metal.Machine]
		ReservedMachines  []string // TODO
	}
)
