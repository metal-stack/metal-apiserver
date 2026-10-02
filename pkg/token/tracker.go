// Package tokentracker provides a low-overhead way to record "last accessed"
// timestamps for JWT tokens stored in Redis, without adding synchronous
// Redis round-trips to the hot request path.
//
// Design:
//   - RecordAccess() is called on every token validation. It's O(1) and
//     touches only an in-memory map — no Redis call on the hot path.
//   - A per-token throttle window (default 30s) means repeated accesses to
//     the same hot token don't generate repeated pending writes.
//   - A background goroutine flushes pending updates on a ticker, using a
//     single pipelined Redis call for the whole batch.
//   - Each token is stored as a Hash (payload + last_access fields) under
//     one key, so a single TTL set at creation time governs both fields.
//   - By default the tracker only ever HSETs last_access — it never calls
//     EXPIRE — so last_access always expires exactly when the token does,
//     with no drift. See WithSlidingExpiry for opaque (non-JWT) tokens
//     where you actually want idle-based expiry instead.
package token

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	// PayloadField is the hash field holding the token's stored value/claims.
	PayloadField = "payload"
	// LastAccessField is the hash field holding the last-access unix timestamp.
	LastAccessField = "last_access"
)

// Tracker batches and throttles last-access writes for tokens stored as
// Redis hashes under key pattern "token:{<id>}".
type Tracker struct {
	client        valkey.Client
	ttl           time.Duration // only used if slidingExpiry is true
	slidingExpiry bool          // if true, refresh TTL to ttl on every flushed access
	window        time.Duration // minimum time between recorded updates per token

	mu      sync.Mutex
	pending map[string]time.Time // tokenID -> access time waiting to be flushed
	lastSet map[string]time.Time // tokenID -> last time we actually flushed (for throttling)

	flushInterval time.Duration
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// Option configures a Tracker.
type Option func(*Tracker)

// WithTTL only has an effect when combined with WithSlidingExpiry(true).
// By default the tracker never sets TTL — it relies on the token's own
// TTL (set at creation, e.g. from the JWT's exp claim) staying untouched.
func WithTTL(d time.Duration) Option { return func(t *Tracker) { t.ttl = d } }

// WithSlidingExpiry makes the tracker re-issue EXPIRE(ttl) on every
// flushed access, turning last-access tracking into idle-expiry too.
// Do NOT enable this for JWTs whose validity is defined by an `exp`
// claim — it will desynchronize the Redis TTL from the token's real
// expiry. Only use it for opaque session tokens with no fixed expiry
// of their own.
func WithSlidingExpiry(enabled bool) Option { return func(t *Tracker) { t.slidingExpiry = enabled } }

func WithThrottleWindow(d time.Duration) Option { return func(t *Tracker) { t.window = d } }
func WithFlushInterval(d time.Duration) Option  { return func(t *Tracker) { t.flushInterval = d } }

// New creates a Tracker and starts its background flush loop.
// Call Stop() during shutdown to flush remaining updates.
func New(client valkey.Client, opts ...Option) *Tracker {
	t := &Tracker{
		client:        client,
		slidingExpiry: false, // default: never touch TTL, rely on the token's own expiry
		window:        30 * time.Second,
		flushInterval: 5 * time.Second,
		pending:       make(map[string]time.Time),
		lastSet:       make(map[string]time.Time),
		stopCh:        make(chan struct{}),
	}
	for _, opt := range opts {
		opt(t)
	}

	t.wg.Add(1)
	go t.flushLoop()
	return t
}

// redisKey builds the hash key for a token. The {} hash tag ensures the
// token's key and any related keys land in the same Redis Cluster slot.
func redisKey(tokenID string) string {
	return "token:{" + tokenID + "}"
}

// RecordAccess should be called on every successful token validation.
// It is cheap: a mutex lock + map write, no I/O. Safe for concurrent use.
func (t *Tracker) RecordAccess(tokenID string) {
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()

	// Throttle: skip if we already flushed (or scheduled) an update for
	// this token within the window. This is what keeps hot tokens cheap.
	if last, ok := t.lastSet[tokenID]; ok && now.Sub(last) < t.window {
		return
	}

	t.pending[tokenID] = now
}

// flushLoop periodically writes all pending access times in one pipeline.
func (t *Tracker) flushLoop() {
	defer t.wg.Done()
	ticker := time.NewTicker(t.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.flush()
		case <-t.stopCh:
			t.flush() // final flush on shutdown
			return
		}
	}
}

// flush drains the pending map and writes it to Redis using a single
// pipeline (one network round trip regardless of batch size).
func (t *Tracker) flush() {
	t.mu.Lock()
	if len(t.pending) == 0 {
		t.mu.Unlock()
		return
	}
	batch := t.pending
	t.pending = make(map[string]time.Time, len(batch))
	t.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// valkey-go pipelines by building a slice of commands and sending them
	// together with DoMulti — one network round trip for the whole batch,
	// same idea as go-redis's Pipeline but without a Pipeliner object.
	//
	// IMPORTANT: we intentionally do NOT call EXPIRE here. HSET never
	// touches a key's existing TTL — so as long as the hash's TTL was set
	// correctly at creation time (see StoreToken below, deriving it from
	// the JWT's own `exp` claim), last_access rides along on that same
	// TTL and disappears exactly when the token does. Re-issuing EXPIRE
	// on every access would silently turn this into sliding expiration
	// and could extend or shorten a token's real (JWT-defined) lifetime
	// as a side effect of tracking — see WithSlidingExpiry if you
	// actually want that behavior for non-JWT session tokens.
	cmds := make([]valkey.Completed, 0, len(batch))
	for tokenID, ts := range batch {
		key := redisKey(tokenID)
		cmds = append(cmds,
			t.client.B().Hset().Key(key).
				FieldValue().FieldValue(LastAccessField, strconv.FormatInt(ts.Unix(), 10)).
				Build(),
		)
		if t.slidingExpiry {
			cmds = append(cmds, t.client.B().Expire().Key(key).Seconds(int64(t.ttl.Seconds())).Build())
		}
	}

	var flushErr error
	for _, res := range t.client.DoMulti(ctx, cmds...) {
		if err := res.Error(); err != nil {
			flushErr = err // keep going; report the last error, all results are best-effort
		}
	}
	if flushErr != nil {
		// Writes are best-effort: log and move on. Don't block or retry
		// synchronously — losing a last-access update is acceptable,
		// blocking the flush loop is not.
		log.Printf("tokentracker: flush had errors for %d tokens (last: %v)", len(batch), flushErr)
		return
	}

	// Only record lastSet on success, so a failed flush gets retried
	// on the token's next access rather than silently throttled away.
	now := time.Now()
	t.mu.Lock()
	for tokenID := range batch {
		t.lastSet[tokenID] = now
	}
	t.mu.Unlock()
}

// Stop flushes any remaining pending updates and stops the background loop.
// Call this during graceful shutdown.
func (t *Tracker) Stop() {
	close(t.stopCh)
	t.wg.Wait()
}

// --- Example usage in a token validation path ---
//
// func ValidateToken(ctx context.Context, client valkey.Client, tracker *tokentracker.Tracker, tokenID string) (claims string, err error) {
//     key := "token:{" + tokenID + "}"
//     val, err := client.Do(ctx, client.B().Hget().Key(key).Field(tokentracker.PayloadField).Build()).ToString()
//     if err != nil {
//         return "", err // not found / expired / valkey error
//     }
//
//     // Hot path: this is O(1) in-memory, no extra network round trip.
//     tracker.RecordAccess(tokenID)
//
//     return val, nil
// }
//
// Storing a new token (e.g. at login). ttl should be derived from the
// JWT's own `exp` claim (e.g. time.Until(claims.ExpiresAt.Time)) so the
// Redis key — and therefore last_access, which lives in the same hash —
// expires at exactly the moment the token itself becomes invalid:
//
// func StoreToken(ctx context.Context, client valkey.Client, tokenID, payload string, ttl time.Duration) error {
//     key := "token:{" + tokenID + "}"
//     cmds := []valkey.Completed{
//         client.B().Hset().Key(key).
//             FieldValue().
//             FieldValue(tokentracker.PayloadField, payload).
//             FieldValue(tokentracker.LastAccessField, strconv.FormatInt(time.Now().Unix(), 10)).
//             Build(),
//         client.B().Expire().Key(key).Seconds(int64(ttl.Seconds())).Build(),
//     }
//     for _, res := range client.DoMulti(ctx, cmds...) {
//         if err := res.Error(); err != nil {
//             return err
//         }
//     }
//     return nil
// }
//
// From here on, Tracker.RecordAccess + the background flush only ever
// HSET the last_access field — never EXPIRE — so this TTL is left alone
// until the token's natural expiry deletes the whole hash, field and all.
//
// Client setup:
//
// client, err := valkey.NewClient(valkey.ClientOption{
//     InitAddress: []string{"127.0.0.1:6379"},
// })
// if err != nil {
//     log.Fatal(err)
// }
// defer client.Close()
// tracker := tokentracker.New(client)
// defer tracker.Stop()
