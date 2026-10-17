package metal

import (
	"time"
)

// Base implements common fields for most basic entity types (not all).
type Base struct {
	ID          string    `rethinkdb:"id,omitempty" json:"id"`
	Name        string    `rethinkdb:"name" json:"name"`
	Description string    `rethinkdb:"description" json:"description"`
	Created     time.Time `rethinkdb:"created" json:"created"`
	Changed     time.Time `rethinkdb:"changed" json:"changed"`
	Generation  uint64    `rethinkdb:"generation" json:"generation"`
}

// GetID returns the ID of the entity
func (b *Base) GetID() string {
	return b.ID
}

// SetID sets the ID of the entity
func (b *Base) SetID(id string) {
	b.ID = id
}

// GetChanged returns the last changed timestamp of the entity
func (b *Base) GetChanged() time.Time {
	return b.Changed
}

// GetCreated returns the creation timestamp of the entity
func (b *Base) GetCreated() time.Time {
	return b.Created
}

// GetGeneration returns the creation timestamp of the entity
func (b *Base) GetGeneration() uint64 {
	return b.Generation
}

func (b *Base) SetChanged(t time.Time) {
	b.Changed = t
}
