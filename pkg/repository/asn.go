package repository

import (
	"context"
	"fmt"

	"github.com/metal-stack/api/go/errorutil"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/async/task"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/db/queries"
)

const (
	// ASNMin is the minimum asn defined according to
	// https://en.wikipedia.org/wiki/Autonomous_system_(Internet)
	ASNMin = uint32(4200000000)

	// ASNBase is the offset for all Machine ASN´s
	ASNBase = uint32(4210000000)

	// ASNMax defines the maximum allowed asn
	// https://en.wikipedia.org/wiki/Autonomous_system_(Internet)
	ASNMax = uint32(4294967294)
)

// acquireASN fetches a unique integer by using the existing integer pool and adding to ASNBase
func (r *machineRepository) acquireASN(ctx context.Context) (*uint32, error) {
	i, err := r.s.ds.AsnPool().AcquireRandomUniqueInteger(ctx)
	if err != nil {
		return nil, err
	}

	asn := ASNBase + uint32(i)
	if asn > ASNMax {
		return nil, fmt.Errorf("unable to calculate asn, got a asn larger than ASNMax: %d > %d", asn, ASNMax)
	}

	return &asn, nil
}

// releaseASN will release the asn from the integerpool
func (r *machineRepository) releaseASN(ctx context.Context, asn uint32) error {
	if asn < ASNBase || asn > ASNMax {
		return fmt.Errorf("asn %d might not be smaller than:%d or larger than %d", asn, ASNBase, ASNMax)
	}

	i := uint(asn - ASNBase)

	return r.s.ds.AsnPool().ReleaseUniqueInteger(ctx, i)
}

func (r *machineRepository) releaseAsnTask(ctx context.Context, payload *task.MachineDeletePayload) error {
	r.s.log.Debug("machine delete attempting to release asn")

	m, err := r.s.ds.Machine().Find(ctx, queries.MachineFilter(&apiv2.MachineQuery{
		Allocation: &apiv2.MachineAllocationQuery{
			Uuid: &payload.AllocationUUID,
		},
	}))
	if err != nil {
		if errorutil.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("unable to find machine %q: %w", payload.AllocationUUID, err)
	}

	var asn uint32

	for _, nw := range m.Allocation.MachineNetworks {
		switch nw.NetworkType {
		case metal.NetworkTypeChild, metal.NetworkTypeChildShared:
			if asn >= ASNBase {
				asn = nw.ASN
			}
		}

		if asn > 0 {
			break
		}
	}

	if asn == 0 {
		return nil
	}

	// if in the meantime someone else allocated the asn, we do not want to erase it
	// so check if it's used somewhere else

	machines, err := r.s.UnscopedMachine().List(ctx, &apiv2.MachineQuery{
		Network: &apiv2.MachineNetworkQuery{
			Asns: []uint32{asn},
		},
		Allocation: &apiv2.MachineAllocationQuery{},
	})
	if err != nil {
		return fmt.Errorf("unable to list machines: %w", err)
	}

	for _, otherMachine := range machines {
		if otherMachine.Allocation.Uuid != payload.AllocationUUID {
			// somebody else has it, do not release
			return nil
		}
	}

	// release asn does an insert with replace, so this is already idempotent and needs no further checking
	if err := r.s.UnscopedMachine().AdditionalMethods().releaseASN(ctx, asn); err != nil {
		return fmt.Errorf("unable to release asn: %w", err)
	}

	r.s.log.Debug("machine delete released asn", "asn", asn)

	return nil
}
