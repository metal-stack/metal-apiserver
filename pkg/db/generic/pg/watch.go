package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/lib/pq"
)

// NotifyChannel is the PostgreSQL channel on which every change to any
// per-entity table is announced. It is kept in sync with the trigger
// created by RepositorySchema.
const NotifyChannel = "entity_table_changes"

// ErrWatchNotConfigured is returned when Watch is called on a repository that
// was not given a Watcher. Postgres watch support requires a dedicated
// LISTEN connection, which is not available from a plain *sql.DB.
var ErrWatchNotConfigured = errors.New("watch is not configured: no postgres watcher was provided")

type (
	// Change describes a single change to an entity as announced through
	// PostgreSQL NOTIFY. New carries the raw JSON of the entity as it looks
	// after the change and is nil for deletions.
	Change struct {
		EntityType string
		EntityID   string
		Op         string
		New        []byte
	}

	subscriber struct {
		ch chan Change

		mu     sync.Mutex
		closed bool
	}

	// Watcher is a shared PostgreSQL LISTEN client. A single Watcher serves
	// every repository built on the same database: it listens on NotifyChannel
	// once, decodes the notification payload and fans the change out to the
	// matching subscriptions.
	//
	// A Watcher must be created with NewWatcher and closed with Close when it
	// is no longer needed.
	Watcher struct {
		log      *slog.Logger
		db       *sql.DB
		listener *pq.Listener

		mu   sync.RWMutex
		subs map[watchKey]map[*subscriber]struct{}
	}

	watchKey struct {
		entityType string
		id         string
	}
)

// NewWatcher opens a dedicated LISTEN connection using dsn and starts
// dispatching notifications to subscribers. db is used to load the entity
// state after a change was announced. The caller owns the Watcher and must
// close it.
func NewWatcher(log *slog.Logger, db *sql.DB, dsn string) (*Watcher, error) {
	if log == nil {
		log = slog.Default()
	}

	listener := pq.NewListener(dsn, 10*time.Second, time.Minute, func(event pq.ListenerEventType, err error) {
		switch event {
		case pq.ListenerEventConnected, pq.ListenerEventReconnected:
			log.Debug("postgres listener connected", "event", event.String())
		default:
			if err != nil {
				log.Error("postgres listener event", "event", event.String(), "error", err)
			}
		}
	})

	w := &Watcher{
		log:      log.WithGroup("generic-watcher"),
		db:       db,
		listener: listener,
		subs:     map[watchKey]map[*subscriber]struct{}{},
	}

	if err := listener.Listen(NotifyChannel); err != nil {
		_ = listener.Close()
		return nil, err
	}

	go w.dispatch()

	return w, nil
}

// Close stops listening and releases the dedicated connection.
func (w *Watcher) Close() error {
	if w == nil {
		return nil
	}
	return w.listener.Close()
}

// Subscribe registers interest in changes of a single entity. The returned
// channel is closed when ctx is cancelled.
func (w *Watcher) Subscribe(ctx context.Context, entityType, id string) (<-chan Change, error) {
	if w == nil {
		return nil, ErrWatchNotConfigured
	}

	sub := &subscriber{ch: make(chan Change, 64)}
	key := watchKey{entityType: entityType, id: id}

	w.mu.Lock()
	if w.subs[key] == nil {
		w.subs[key] = map[*subscriber]struct{}{}
	}
	w.subs[key][sub] = struct{}{}
	w.mu.Unlock()

	go func() {
		<-ctx.Done()

		w.mu.Lock()
		delete(w.subs[key], sub)
		if len(w.subs[key]) == 0 {
			delete(w.subs, key)
		}
		w.mu.Unlock()

		sub.close()
	}()

	return sub.ch, nil
}

func (w *Watcher) dispatch() {
	for notification := range w.listener.NotificationChannel() {
		if notification == nil {
			// The listener signals a reconnect with a nil notification.
			continue
		}

		var payload struct {
			ID         string `json:"id"`
			EntityType string `json:"entity_type"`
			Op         string `json:"op"`
		}
		if err := json.Unmarshal([]byte(notification.Extra), &payload); err != nil {
			w.log.Error("unable to decode notification payload", "payload", notification.Extra, "error", err)
			continue
		}

		w.notify(payload.ID, payload.EntityType, payload.Op)
	}
}

func (w *Watcher) notify(id, entityType, op string) {
	key := watchKey{entityType: entityType, id: id}

	w.mu.RLock()
	subs := make([]*subscriber, 0, len(w.subs[key]))
	for sub := range w.subs[key] {
		subs = append(subs, sub)
	}
	w.mu.RUnlock()

	if len(subs) == 0 {
		return
	}

	var newData []byte
	if op != "DELETE" {
		// With a table per entity type, entityType is also the table name (the
		// same identifier the repository writes to).
		query := `SELECT data FROM ` + entityType + ` WHERE id = $1`
		err := w.db.QueryRowContext(context.Background(), query, id).Scan(&newData)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			w.log.Error("unable to load changed entity", "id", id, "entity_type", entityType, "error", err)
			return
		}
	}

	change := Change{EntityType: entityType, EntityID: id, Op: op, New: newData}
	for _, sub := range subs {
		sub.send(change)
	}
}

// send delivers a change without ever blocking the dispatch loop. If the
// consumer is not keeping up the change is dropped: every change carries the
// full new state, so a later change supersedes it.
func (s *subscriber) send(c Change) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- c:
	default:
	}
}

func (s *subscriber) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.ch)
}
