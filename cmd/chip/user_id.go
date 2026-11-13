package main

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"
)

const (
	userIDHeader              = "x-mcp-user-id"
	currentUserPath           = "/rest/2.0/users/current"
	userIDCacheTTL            = 10 * time.Minute
	userIDTransientFailureTTL = 30 * time.Second
	userIDCacheSize           = 1000
	userIDLookupTimeout       = 500 * time.Millisecond
	userIDResponseLimit       = 1 << 20
)

// userIDResolver maps an Authorization value to the DGC user UUID it authenticates as. Results,
// including failed lookups (cached as an empty ID), are cached so a credential costs at most one
// lookup per TTL however many requests it makes. A successful lookup, and a failure that a retry
// would repeat, are kept for userIDCacheTTL; a transient failure is kept for
// userIDTransientFailureTTL so a struggling DGC is not sent a lookup with every request but the
// header comes back soon after it recovers. The lookup timeout is short because the first request
// on a credential waits for it and the header only serves attribution.
type userIDResolver struct {
	next          http.RoundTripper
	lookupTimeout time.Duration
	cache         *userIDCache
}

func newUserIDResolver(next http.RoundTripper) *userIDResolver {
	return &userIDResolver{
		next:          next,
		lookupTimeout: userIDLookupTimeout,
		cache:         newUserIDCache(userIDCacheSize, userIDCacheTTL, time.Now),
	}
}

// resolve returns the user UUID for authorization, or "" when it is unknown. It waits for an
// in-flight lookup of the same credential rather than starting another one, unless ctx ends first.
func (r *userIDResolver) resolve(ctx context.Context, baseURL *url.URL, authorization string) string {
	key := credentialKey(authorization)
	entry, owner := r.cache.acquire(key)
	if owner {
		r.fill(ctx, baseURL, authorization, entry)
	}
	select {
	case <-entry.done:
		return entry.id
	case <-ctx.Done():
		return ""
	}
}

// fill looks the ID up, logs a failure and completes entry with the TTL its outcome calls for. A
// panic in the lookup is recovered as a transient failure, so entry is always completed and the
// requests waiting on it are released.
func (r *userIDResolver) fill(ctx context.Context, baseURL *url.URL, authorization string, entry *userIDEntry) {
	var id string
	var failure *lookupFailure
	defer func() {
		if recovered := recover(); recovered != nil {
			id, failure = "", transientFailure(fmt.Errorf("lookup panicked: %v", recovered))
		}
		if failure != nil {
			logLookupFailure(ctx, failure)
		}
		r.cache.complete(entry, id, failure.cacheTTL())
	}()

	id, failure = r.lookup(ctx, baseURL, authorization)
}

// lookup calls the inner transport directly rather than clients.GetCurrentUser, which goes
// through collibraClient and would recurse back into this resolver. It is detached from the
// caller's cancellation because its result is shared with every other request on the same
// credential.
func (r *userIDResolver) lookup(ctx context.Context, baseURL *url.URL, authorization string) (string, *lookupFailure) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.lookupTimeout)
	defer cancel()

	lookupURL := *baseURL
	lookupURL.Path = path.Join(baseURL.Path, currentUserPath)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, lookupURL.String(), nil)
	if err != nil {
		return "", transientFailure(err)
	}
	request.Header.Set("Authorization", authorization)
	request.Header.Set("Accept", "application/json")

	response, err := r.next.RoundTrip(request)
	if err != nil {
		return "", transientFailure(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, userIDResponseLimit))
		return "", statusFailure(response.StatusCode)
	}
	return decodeUserID(response.Body)
}

// decodeUserID reads at most userIDResponseLimit bytes; a larger body is treated as undecodable.
func decodeUserID(body io.Reader) (string, *lookupFailure) {
	data, err := io.ReadAll(io.LimitReader(body, userIDResponseLimit+1))
	if err != nil {
		return "", transientFailure(fmt.Errorf("reading response: %w", err))
	}
	if len(data) > userIDResponseLimit {
		return "", transientFailure(fmt.Errorf("response exceeds %d bytes", userIDResponseLimit))
	}
	var user struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &user); err != nil {
		return "", transientFailure(fmt.Errorf("decoding response: %w", err))
	}
	if user.ID == "" {
		return "", &lookupFailure{err: fmt.Errorf("response has no id")}
	}
	return user.ID, nil
}

// lookupFailure is a failed lookup. A transient failure may succeed on retry; any other failure
// means the credential cannot be resolved and retrying would repeat it.
type lookupFailure struct {
	err       error
	status    int
	transient bool
}

func transientFailure(err error) *lookupFailure {
	return &lookupFailure{err: err, transient: true}
}

// statusFailure treats a 4xx as definitive except 408 and 429, which ask the caller to retry.
func statusFailure(status int) *lookupFailure {
	retryable := status < 400 || status >= 500 || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests
	return &lookupFailure{err: fmt.Errorf("status %d", status), status: status, transient: retryable}
}

// cacheTTL returns how long the outcome is cached; a nil failure is a successful lookup.
func (f *lookupFailure) cacheTTL() time.Duration {
	if f != nil && f.transient {
		return userIDTransientFailureTTL
	}
	return userIDCacheTTL
}

// logLookupFailure logs a rejected credential at debug level since it is an expected outcome for
// a client that sends a credential DGC does not accept.
func logLookupFailure(ctx context.Context, failure *lookupFailure) {
	level := slog.LevelWarn
	if failure.status == http.StatusUnauthorized || failure.status == http.StatusForbidden {
		level = slog.LevelDebug
	}
	slog.Log(ctx, level, fmt.Sprintf("Could not resolve the current user, %s will not be sent: %v", userIDHeader, failure.err))
}

// credentialKey hashes the credential so the cache does not keep secrets in memory.
func credentialKey(authorization string) [sha256.Size]byte {
	return sha256.Sum256([]byte(authorization))
}

// userIDCache is a bounded, least-recently-used cache whose entries expire the TTL given to
// complete after their lookup completes.
type userIDCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	now      func() time.Time
	order    *list.List
	entries  map[[sha256.Size]byte]*list.Element
}

type userIDEntry struct {
	key     [sha256.Size]byte
	id      string
	expires time.Time
	done    chan struct{}
}

func newUserIDCache(capacity int, ttl time.Duration, now func() time.Time) *userIDCache {
	return &userIDCache{
		capacity: capacity,
		ttl:      ttl,
		now:      now,
		order:    list.New(),
		entries:  make(map[[sha256.Size]byte]*list.Element),
	}
}

// acquire returns the live entry for key, or a new pending one. owner is true when the caller
// created the entry and must look the ID up and call complete.
func (c *userIDCache) acquire(key [sha256.Size]byte) (entry *userIDEntry, owner bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.entries[key]; ok {
		entry = element.Value.(*userIDEntry)
		if c.now().Before(entry.expires) {
			c.order.MoveToFront(element)
			return entry, false
		}
		c.remove(element)
	}

	// A pending entry's expiry only has to outlast the lookup; complete replaces it.
	entry = &userIDEntry{key: key, expires: c.now().Add(c.ttl), done: make(chan struct{})}
	c.entries[key] = c.order.PushFront(entry)
	for c.order.Len() > c.capacity {
		c.remove(c.order.Back())
	}
	return entry, true
}

func (c *userIDCache) complete(entry *userIDEntry, id string, ttl time.Duration) {
	c.mu.Lock()
	entry.id = id
	entry.expires = c.now().Add(ttl)
	c.mu.Unlock()
	close(entry.done)
}

func (c *userIDCache) remove(element *list.Element) {
	c.order.Remove(element)
	delete(c.entries, element.Value.(*userIDEntry).key)
}
