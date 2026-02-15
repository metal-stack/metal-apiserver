package scenarios

import (
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

type (
	DatacenterSpec struct {
		Tenants           []string
		ProjectsPerTenant int
		Partitions        map[string]Partition
		Images            map[string]apiv2.ImageFeature
		Sizes             []*apiv2.Size
		SizeReservations  []*adminv2.SizeReservationServiceCreateRequest
		Networks          []*adminv2.NetworkServiceCreateRequest
		IPs               []*apiv2.IPServiceCreateRequest
	}

	Partition struct {
		Racks map[string]Rack
	}

	Rack struct {
		Switches []*apiv2.Switch
		Machines []*metal.Machine
	}
)
