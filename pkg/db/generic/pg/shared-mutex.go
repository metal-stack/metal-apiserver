package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	// SharedMutexSchema creates the table used by the shared mutex. A row with
	// the mutex key exists as long as the mutex is held and is deleted on
	// release. Postgres' primary key constraint provides the atomic "insert
	// only if not present" guarantee the mutex relies on.
	SharedMutexSchema = `
CREATE TABLE IF NOT EXISTS shared_mutex (
    key        VARCHAR(255) PRIMARY KEY,
    locked_at  TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_shared_mutex_expires_at ON shared_mutex (expires_at);
`

	// defaultSharedMutexAcquireTimeout defines a timeout for the context for the acquisition of the mutex.
	defaultSharedMutexAcquireTimeout = 10 * time.Second
	// defaultSharedMutexExpirationTimeout defines when mutexes are considered expired and will be cleaned up by the next check interval.
	defaultSharedMutexExpirationTimeout = 10 * time.Second
	// defaultSharedMutexRetryInterval defines how often an acquirer that lost the race retries to acquire the mutex.
	// this is the postgres equivalent of listening for the release change.
	defaultSharedMutexRetryInterval = 100 * time.Millisecond
	// defaultSharedMutexCheckInterval is the interval in which it is checked whether mutexes have expired.
	// if they have expired, they will be released. this is a safety mechanism in case a mutex was forgotten
	// to be released to prevent the whole machinery to lock up forever.
	defaultSharedMutexCheckInterval = 30 * time.Second
)

type (
	// SharedMutex constructs a mutex using postgres to guarantee atomic
	// operations. this can be helpful to prevent concurrency issues over
	// multiple metal-api replicas.
	// the performance of this is remarkably worse than running code without
	// this mutex, so only make use of this when it really makes sense.
	SharedMutex struct {
		db            *sql.DB
		retryinterval time.Duration
		checkinterval time.Duration
		log           *slog.Logger
	}

	sharedMutexDoc struct {
		Key       string    `json:"key"`
		LockedAt  time.Time `json:"locked_at"`
		ExpiresAt time.Time `json:"expires_at"`
	}

	mutexOpt any

	mutexOptCheckInterval struct {
		interval time.Duration
	}

	lockOpt any

	lockOptAcquireTimeout struct {
		timeout time.Duration
	}

	lockOptExpirationTimeout struct {
		expiration time.Duration
	}
)

// NewMutexOptCheckInterval sets the interval in which the expiration loop
// checks for expired mutexes.
func NewMutexOptCheckInterval(interval time.Duration) *mutexOptCheckInterval {
	return &mutexOptCheckInterval{interval: interval}
}

// NewSharedMutex creates a shared mutex backed by the given postgres
// connection and starts its expiration loop. The loop is stopped when ctx is
// cancelled.
func NewSharedMutex(ctx context.Context, log *slog.Logger, db *sql.DB, opts ...mutexOpt) (*SharedMutex, error) {
	if _, err := db.ExecContext(context.Background(), SharedMutexSchema); err != nil {
		return nil, err
	}

	checkinterval := defaultSharedMutexCheckInterval

	for _, opt := range opts {
		switch o := opt.(type) {
		case *mutexOptCheckInterval:
			checkinterval = o.interval
		default:
			return nil, fmt.Errorf("unknown option: %T", opt)
		}
	}

	m := &SharedMutex{
		log:           log.WithGroup("shared-mutex"),
		db:            db,
		retryinterval: defaultSharedMutexRetryInterval,
		checkinterval: checkinterval,
	}

	go m.expireloop(ctx)

	return m, nil
}

// NewLockOptAcquireTimeout sets the maximum time to wait for the mutex to be
// released before giving up.
func NewLockOptAcquireTimeout(t time.Duration) *lockOptAcquireTimeout {
	return &lockOptAcquireTimeout{timeout: t}
}

// NewLockOptExpirationTimeout sets how long the mutex is held before it is
// considered expired and released by the expiration loop.
func NewLockOptExpirationTimeout(expiration time.Duration) *lockOptExpirationTimeout {
	return &lockOptExpirationTimeout{expiration: expiration}
}

// Lock acquires the mutex for the given key. It returns an error if the mutex
// could not be acquired within the acquire timeout.
func (m *SharedMutex) Lock(ctx context.Context, key string, opts ...lockOpt) error {
	var (
		expiration = defaultSharedMutexExpirationTimeout
		timeout    = defaultSharedMutexAcquireTimeout
	)

	for _, opt := range opts {
		switch o := opt.(type) {
		case *lockOptAcquireTimeout:
			timeout = o.timeout
		case *lockOptExpirationTimeout:
			expiration = o.expiration
		default:
			return fmt.Errorf("unknown lock option: %T", opt)
		}
	}

	locked, err := m.tryLock(ctx, key, expiration)
	if err != nil {
		return err
	}
	if locked {
		m.log.Debug("mutex acquired", "key", key)
		return nil
	}

	m.log.Debug("mutex is already locked, waiting for release", "key", key)

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("unable to acquire mutex: %s", key)
		case <-time.After(m.retryinterval):
		}

		locked, err := m.tryLock(timeoutCtx, key, expiration)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return fmt.Errorf("unable to acquire mutex: %s", key)
			}
			return err
		}
		if !locked {
			m.log.Debug("mutex was not yet released", "key", key)
			continue
		}

		m.log.Debug("mutex acquired after waiting", "key", key)
		return nil
	}
}

// Unlock releases the mutex for the given key.
func (m *SharedMutex) Unlock(ctx context.Context, key string) {
	const query = `DELETE FROM shared_mutex WHERE key = $1`

	if _, err := m.db.ExecContext(ctx, query, key); err != nil {
		m.log.Error("unable to release shared mutex", "key", key, "error", err)
	}
}

// tryLock attempts to insert the mutex row atomically. It reports whether the
// mutex was acquired. A nil error with locked==false means another holder owns
// the mutex.
func (m *SharedMutex) tryLock(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	const query = `
		INSERT INTO shared_mutex (key, locked_at, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO NOTHING
	`

	var (
		now = time.Now()
		doc = &sharedMutexDoc{
			Key:       key,
			LockedAt:  now,
			ExpiresAt: now.Add(expiration),
		}
	)

	res, err := m.db.ExecContext(ctx, query, doc.Key, doc.LockedAt, doc.ExpiresAt)
	if err != nil {
		return false, err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return rows == 1, nil
}

func (m *SharedMutex) expireloop(ctx context.Context) {
	const query = `DELETE FROM shared_mutex WHERE expires_at < NOW()`

	ticker := time.NewTicker(m.checkinterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.log.Debug("checking for expired mutexes")

			res, err := m.db.ExecContext(ctx, query)
			if err != nil {
				m.log.Error("unable to release shared mutexes", "error", err)
				continue
			}

			deleted, err := res.RowsAffected()
			if err != nil {
				m.log.Error("unable to read deletion count", "error", err)
				continue
			}

			m.log.Debug("searched for expiring mutexes in database", "deletion-count", deleted)
		case <-ctx.Done():
			m.log.Info("stopped shared mutex expiration loop")
			return
		}
	}
}
