package routing

import (
	"errors"
	"reflect"

	"github.com/metal-stack/metal-apiserver/pkg/db/generic"
)

// clone makes a shallow copy of a pointer entity. It is used by the dual-write
// router so the two adapters do not mutate each other's view of the entity
// (the RethinkDB adapter stamps changed/generation in place).
//
// A shallow copy is sufficient because the adapters only mutate top-level
// fields; nested structs are never modified while writing.
func clone[E generic.Entity](e E) (E, error) {
	v := reflect.ValueOf(e)
	if v.Kind() != reflect.Pointer {
		// Value entities are already copied by assignment.
		return e, nil
	}
	if v.IsNil() {
		var zero E
		return zero, errors.New("cannot clone a nil entity")
	}

	c := reflect.New(v.Type().Elem())
	c.Elem().Set(v.Elem())

	return c.Interface().(E), nil
}
