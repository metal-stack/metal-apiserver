package postgres

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultPgLockAcquireTimeout = 10 * time.Second
)

type (
	lockOpt any

	lockOptAcquireTimeout struct {
		timeout time.Duration
	}
)

func NewLockOptAcquireTimeout(t time.Duration) *lockOptAcquireTimeout {
	return &lockOptAcquireTimeout{timeout: t}
}

// pgLocker implements distributed locking using PostgreSQL advisory locks.
type pgLocker struct {
	log  *slog.Logger
	pool *pgxpool.Pool
}

func newPgLocker(log *slog.Logger, pool *pgxpool.Pool) *pgLocker {
	return &pgLocker{
		log:  log,
		pool: pool,
	}
}

func lockKey(key string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(key))
	return h.Sum64()
}

func (l *pgLocker) lock(ctx context.Context, key string, opts ...any) error {
	var timeout = defaultPgLockAcquireTimeout

	for _, opt := range opts {
		switch o := opt.(type) {
		case *lockOptAcquireTimeout:
			timeout = o.timeout
		}
	}

	lk := lockKey(key)

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		var acquired bool
		err := l.pool.QueryRow(timeoutCtx,
			`SELECT pg_try_advisory_lock($1)`,
			int64(lk),
		).Scan(&acquired)
		if err != nil {
			return fmt.Errorf("cannot acquire advisory lock for key %q: %w", key, err)
		}
		if acquired {
			l.log.Debug("advisory lock acquired", "key", key)
			return nil
		}

		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("unable to acquire lock: %s", key)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (l *pgLocker) unlock(ctx context.Context, key string) {
	lk := lockKey(key)
	_, err := l.pool.Exec(ctx, `SELECT pg_advisory_unlock($1)`, int64(lk))
	if err != nil {
		l.log.Error("unable to release advisory lock", "key", key, "error", err)
	}
}
