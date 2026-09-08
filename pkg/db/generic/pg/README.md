# Generic Postgres Datastore

Provides two interfaces, one for Entity CRUD and one for Integer Pool.

Main difference of the Entity CRUD compared to what we actually have with rethinkdb:

1. ID must be a UUIDv7, this will hurt for Entities which actually have a non-UUID id:

  - IP
  - Size
  - Partition
  - Image
  - Filesystemlayout

These Entities must get a UUIDv7 during migration and Queries must be adopted to search by the Name property of it.
As these Entities are low volume, this should not hurt performance.
Also Name Uniqueness must also be ensured on repository layer, or newly created machines must reference them by uuid instead ?
Could be made possible by checking if the reference is a uuid, otherwise query by name.

2. Queries must be formatted in a different way

```golang
    machineRepo := pg.NewGenericRepository[Machine](db)
    imagePath := pg.PathOf(func(m *Machine) any {
        return &m.Image.Name
    })
    machines, err := repo.Query(ctx, []pg.QueryFilter{
        {Path: imagePath, Op: "=", Value: "debian-13.0.20260812"},
    })
```

## TODO

### Migration helper

This should be done Entity by Entity

### Adopt Test and Datacenter framework

