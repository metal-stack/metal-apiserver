package generic

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/metal-stack/metal-apiserver/pkg/errorutil"
	r "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

type storage[E Entity] struct {
	r         *datastore
	table     r.Term
	tableName string
}

// newStorage creates a new Storage which uses the given database abstraction.
func newStorage[E Entity](re *datastore, tableName string) *storage[E] {
	return &storage[E]{
		r:         re,
		table:     r.DB(re.dbname).Table(tableName),
		tableName: tableName,
	}
}

// Create creates the given entity in the database. in case it is already present, a conflict error will be returned.

// if the ID field of the entity is an empty string, the ID will be generated automatically as UUIDv7.
func (s *storage[E]) Create(ctx context.Context, e E) (E, error) {
	var (
		now  = time.Now()
		zero E
	)

	if err := s.setCreated(now, e); err != nil {
		return zero, err
	}
	if err := s.setChanged(now, e); err != nil {
		return zero, err
	}
	if err := s.setGeneration(0, e); err != nil {
		return zero, err
	}

	// Create a uuidv7 id if an empty string is given
	// this ensures alphabetically ordered uuids by creation date.
	if e.GetID() == "" {
		uid, err := uuid.NewV7()
		if err != nil {
			return zero, err
		}
		e.SetID(uid.String())
	}

	_, err := s.table.Insert(e).RunWrite(s.r.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		if r.IsConflictErr(err) {
			return zero, errorutil.Conflict("cannot create %v in database, entity already exists: %s", s.tableName, e.GetID())
		}
		return zero, fmt.Errorf("cannot create %v in database: %w", s.tableName, err)
	}

	return e, nil
}

// Delete deletes the given entity from the database.
func (s *storage[E]) Delete(ctx context.Context, e E) error {
	_, err := s.table.Get(e.GetID()).Delete().RunWrite(s.r.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		return fmt.Errorf("cannot delete %v with id %q from database: %w", s.tableName, e.GetID(), err)
	}

	return nil
}

// Find attempts to find a single entity from the given set of queries.
//
// if either none or more than one entities were found by the query, an error gets returned.
func (s *storage[E]) Find(ctx context.Context, queries ...EntityQuery) (E, error) {
	query := s.table
	for _, q := range queries {
		query = q(query)
	}
	s.r.log.Debug("find", "table", s.table, "query", query.String())

	var zero E
	res, err := query.Run(s.r.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		return zero, fmt.Errorf("cannot find %v in database: %w", s.tableName, err)
	}
	defer func() {
		if err := res.Close(); err != nil {
			s.r.log.Error("unable to close database connection", "error", err)
		}
	}()
	if res.IsNil() {
		return zero, errorutil.NotFound("no %v found", s.tableName)
	}

	e := new(E)

	hasResult := res.Next(e)
	if !hasResult {
		return zero, errorutil.NotFound("cannot find %v", s.tableName)
	}

	next := map[string]any{}
	hasResult = res.Next(&next)
	if hasResult {
		return zero, fmt.Errorf("more than one %v exists", s.tableName)
	}

	return *e, nil
}

// List returns all entities present in the database, optionally filtered by the given set of queries.
func (s *storage[E]) List(ctx context.Context, queries ...EntityQuery) ([]E, error) {
	query := s.table
	for _, q := range queries {
		if q == nil {
			continue
		}
		query = q(query)
	}

	s.r.log.Debug("list", "table", s.table, "query", query.String())

	res, err := query.Run(s.r.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("cannot search %v in database: %w", s.tableName, err)
	}
	defer func() {
		if err := res.Close(); err != nil {
			s.r.log.Error("unable to close database connection", "error", err)
		}
	}()

	result := new([]E)

	err = res.All(result)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch all entities: %w", err)
	}

	return *result, nil
}

// Get returns the entity of the given ID  from the database.
func (s *storage[E]) Get(ctx context.Context, id string) (E, error) {
	var zero E
	res, err := s.table.Get(id).Run(s.r.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		return zero, fmt.Errorf("cannot find %v with id %q in database: %w", s.tableName, id, err)
	}

	defer func() {
		if err := res.Close(); err != nil {
			s.r.log.Error("unable to close database connection", "error", err)
		}
	}()
	if res.IsNil() {
		return zero, errorutil.NotFound("no %v with id %q found", s.tableName, id)
	}

	e := new(E)
	err = res.One(e)
	if err != nil {
		return zero, fmt.Errorf("more than one %v with same id exists: %w", s.tableName, err)
	}

	return *e, nil
}

// Update updates the entity to the contents of the new entity.
//
// it uses the "changed" timestamp of the old entity to figure out if it was already modified by some other process.
// if this happens a conflict error will be returned.
func (s *storage[E]) Update(ctx context.Context, e E) error {
	if e.GetChanged().IsZero() {
		return fmt.Errorf("cannot update %v (%s): no changed timestamp set on entity", s.tableName, e.GetID())
	}

	changedTimestamp := e.GetChanged()

	if err := s.setChanged(time.Now(), e); err != nil {
		return err
	}

	if err := s.setGeneration(e.GetGeneration()+1, e); err != nil {
		return err
	}

	_, err := s.table.Get(e.GetID()).Replace(func(row r.Term) r.Term {
		return r.Branch(row.Field("changed").Eq(r.Expr(changedTimestamp)), e, r.Error(entityAlreadyModifiedErrorMessage))
	}).RunWrite(s.r.queryExecutor, r.RunOpts{Context: ctx})
	if err != nil {
		if strings.Contains(err.Error(), entityAlreadyModifiedErrorMessage) {
			return errorutil.Conflict("cannot update %v (%s): %s", s.tableName, e.GetID(), entityAlreadyModifiedErrorMessage)
		}

		return fmt.Errorf("cannot update %v (%s): %w", s.tableName, e.GetID(), err)
	}

	return nil
}

// Upsert inserts the given entity into the database, replacing it completely if it is already present.
func (s *storage[E]) Upsert(ctx context.Context, e E) error {
	now := time.Now()

	if e.GetCreated().IsZero() {
		if err := s.setCreated(now, e); err != nil {
			return err
		}
	}

	if err := s.setChanged(now, e); err != nil {
		return err
	}
	if err := s.setGeneration(e.GetGeneration()+1, e); err != nil {
		return err
	}

	res, err := s.table.Insert(e, r.InsertOpts{
		Conflict: "replace",
	}).RunWrite(s.r.queryExecutor)
	if err != nil {
		return fmt.Errorf("cannot upsert %v (%s) in database: %w", s.tableName, e.GetID(), err)
	}

	if e.GetID() == "" && len(res.GeneratedKeys) > 0 {
		e.SetID(res.GeneratedKeys[0])
	}

	return nil
}

// initialize initializes the database storage, it should be called before serving the metal-api
// in order to ensure that tables, pools, permissions are properly initialized
func (s *storage[E]) initialize(ctx context.Context) error {
	return s.r.createTable(ctx, s.tableName)
}

func (s storage[E]) setCreated(time time.Time, e E) error {
	return s.setTimeField("Created", time, e)
}

func (s storage[E]) setChanged(time time.Time, e E) error {
	return s.setTimeField("Changed", time, e)
}

func (s storage[E]) setGeneration(generation uint64, e E) error {
	return s.setUint64Field("Generation", generation, e)
}

func (s storage[E]) setTimeField(fieldName string, desiredTime time.Time, e E) error {
	return s.setField(fieldName, reflect.TypeOf(time.Time{}).Kind(), desiredTime, e)
}

func (s storage[E]) setUint64Field(fieldName string, desired uint64, e E) error {
	return s.setField(fieldName, reflect.Uint64, desired, e)
}

func (s storage[E]) setField(fieldName string, kind reflect.Kind, val any, e E) error {
	var (
		// pointer to struct - addressable
		ps = reflect.ValueOf(e)
		// struct
		st = ps.Elem()
	)

	if st.Kind() == reflect.Struct {
		// exported field
		f := st.FieldByName(fieldName)
		if !f.IsValid() {
			return fmt.Errorf("%s field is no valid of:%s", fieldName, s.tableName)
		}
		// A Value can be changed only if it is
		// addressable and was not obtained by
		// the use of unexported struct fields.
		if !f.CanSet() {
			return fmt.Errorf("%s can not be set on:%s", fieldName, s.tableName)
		}

		// change value of N
		switch f.Kind() {
		case kind:
			f.Set(reflect.ValueOf(val))
		default:
			return fmt.Errorf("time can no be set on:%s.%s", s.tableName, fieldName)
		}
	}
	return nil
}
