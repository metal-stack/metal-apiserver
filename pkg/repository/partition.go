package repository

import (
	"context"

	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
)

type partitionRepository struct {
	r *Store
}

// ValidateCreate implements Partition.
func (p *partitionRepository) ValidateCreate(ctx context.Context, create *adminv2.PartitionServiceCreateRequest) (*Validated[*adminv2.PartitionServiceCreateRequest], error) {
	panic("unimplemented")
}

// ValidateDelete implements Partition.
func (p *partitionRepository) ValidateDelete(ctx context.Context, e *metal.Partition) (*Validated[*metal.Partition], error) {
	panic("unimplemented")
}

// ValidateUpdate implements Partition.
func (p *partitionRepository) ValidateUpdate(ctx context.Context, msg *adminv2.PartitionServiceUpdateRequest) (*Validated[*adminv2.PartitionServiceUpdateRequest], error) {
	panic("unimplemented")
}

// Create implements Partition.
func (p *partitionRepository) Create(ctx context.Context, c *Validated[*adminv2.PartitionServiceCreateRequest]) (*metal.Partition, error) {
	partition, err := p.ConvertToInternal(c.message.Partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	resp, err := p.r.ds.Partition().Create(ctx, partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return resp, nil
}

// Delete implements Partition.
func (p *partitionRepository) Delete(ctx context.Context, e *Validated[*metal.Partition]) (*metal.Partition, error) {
	partition, err := p.Get(ctx, e.message.ID)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	err = p.r.ds.Partition().Delete(ctx, partition)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return partition, nil
}

// Get implements Partition.
func (p *partitionRepository) Get(ctx context.Context, id string) (*metal.Partition, error) {
	partition, err := p.r.ds.Partition().Get(ctx, id)
	if err != nil {
		return nil, errorutil.Convert(err)
	}

	return partition, nil
}

// Update implements Partition.
func (p *partitionRepository) Update(ctx context.Context, msg *Validated[*adminv2.PartitionServiceUpdateRequest]) (*metal.Partition, error) {
	panic("unimplemented")
}

// Find implements Partition.
func (p *partitionRepository) Find(ctx context.Context, query *apiv2.PartitionServiceListRequest) (*metal.Partition, error) {
	panic("unimplemented")
}

// List implements Partition.
func (p *partitionRepository) List(ctx context.Context, query *apiv2.PartitionServiceListRequest) ([]*metal.Partition, error) {
	panic("unimplemented")
}

// MatchScope implements Partition.
func (p *partitionRepository) MatchScope(e *metal.Partition) error {
	panic("unimplemented")
}

// ConvertToInternal implements Partition.
func (p *partitionRepository) ConvertToInternal(msg *apiv2.Partition) (*metal.Partition, error) {
	panic("unimplemented")
}

// ConvertToProto implements Partition.
func (p *partitionRepository) ConvertToProto(e *metal.Partition) (*apiv2.Partition, error) {
	panic("unimplemented")
}
