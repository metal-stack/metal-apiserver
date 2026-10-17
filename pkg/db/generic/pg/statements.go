package pg

import (
	"database/sql"
	"sync"
)

// statementPreparer prepares a fixed set of statements and remembers the first
// error that occurs. A constructor can therefore prepare its statements field
// by field - keeping each query next to the field it belongs to, instead of
// relying on the position in a slice - without an error check after every
// call: as soon as one preparation fails, the remaining ones are skipped and
// close releases the statements prepared before the failure.
type statementPreparer struct {
	db  *sql.DB
	err error

	prepared []*sql.Stmt
}

func newStatementPreparer(db *sql.DB) *statementPreparer {
	return &statementPreparer{db: db}
}

// prepare prepares query. Once a preparation has failed it returns nil without
// contacting the database, so callers can keep assigning their remaining
// fields unconditionally and check err once at the end.
func (p *statementPreparer) prepare(query string) *sql.Stmt {
	if p.err != nil {
		return nil
	}

	stmt, err := p.db.Prepare(query)
	if err != nil {
		p.err = err
		return nil
	}

	p.prepared = append(p.prepared, stmt)
	return stmt
}

// close closes every statement prepared successfully so far. It is meant for
// the error path of a constructor, where a partially built value is discarded.
func (p *statementPreparer) close() {
	for _, stmt := range p.prepared {
		_ = stmt.Close()
	}
}

// cachedStmt returns the statement cached under key in cache, preparing query
// and storing it on a miss. database/sql may hand the same *sql.Stmt to
// concurrent preparers of the same query; in that case only a distinct handle
// is closed, closing the shared statement would break its other holders.
func cachedStmt(cache *sync.Map, db *sql.DB, key, query string) (*sql.Stmt, error) {
	if v, ok := cache.Load(key); ok {
		return v.(*sql.Stmt), nil
	}

	stmt, err := db.Prepare(query)
	if err != nil {
		return nil, err
	}

	if v, loaded := cache.LoadOrStore(key, stmt); loaded {
		if v.(*sql.Stmt) != stmt {
			_ = stmt.Close()
		}
		return v.(*sql.Stmt), nil
	}

	return stmt, nil
}
