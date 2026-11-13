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
	userIDHeader        = "x-mcp-user-id"
	currentUserPath     = "/rest/2.0/users/current"
	userIDCacheTTL      = 10 * time.Minute
	userIDCacheSize     = 1000
	userIDLookupTimeout = 2 * time.Second
)

// userIDResolver maps an Authorization value to the DGC user UUID it authenticates as. Results,
// including failed lookups (cached as an empty ID), are kept for userIDCacheTTL so a credential
// costs at most one lookup per TTL however many requests it makes; caching failures keeps a
// struggling DGC from receiving a lookup with every request. The lookup timeout is short because
// the first request on a credential waits for it and the header only serves attribution.
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
// in-flight lookup of the same credential rather than starting another one.
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

// fill completes entry even if the lookup panics, so requests waiting on it are released.
func (r *userIDResolver) fill(ctx context.Context, baseURL *url.URL, authorization string, entry *userIDEntry) {
	var id string
	defer func() { r.cache.complete(entry, id) }()

	id, err := r.lookup(ctx, baseURL, authorization)
	if err != nil {
		slog.WarnContext(ctx, fmt.Sprintf("Could not resolve the current user, %s will not be sent: %v", userIDHeader, err))
	}
}

// lookup calls the inner transport directly rather than clients.GetCurrentUser, which goes
// through collibraClient and would recurse back into this resolver. It is detached from the
// caller's cancellation because its result is shared with every other request on the same
// credential.
func (r *userIDResolver) lookup(ctx context.Context, baseURL *url.URL, authorization string) (string, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.lookupTimeout)
	defer cancel()

	lookupURL := *baseURL
	lookupURL.Path = path.Join(baseURL.Path, currentUserPath)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, lookupURL.String(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", authorization)
	request.Header.Set("Accept", "application/json")

	response, err := r.next.RoundTrip(request)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, response.Body)
		return "", fmt.Errorf("status %d", response.StatusCode)
	}
	var user struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&user); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	if user.ID == "" {
		return "", fmt.Errorf("response has no id")
	}
	return user.ID, nil
}

// credentialKey hashes the credential so the cache does not keep secrets in memory.
func credentialKey(authorization string) [sha256.Size]byte {
	return sha256.Sum256([]byte(authorization))
}

// userIDCache is a bounded, least-recently-used cache whose entries expire ttl after their
// lookup completes.
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

	// A pending entry's expiry only has to outlast the lookup; complete resets it.
	entry = &userIDEntry{key: key, expires: c.now().Add(c.ttl), done: make(chan struct{})}
	c.entries[key] = c.order.PushFront(entry)
	for c.order.Len() > c.capacity {
		c.remove(c.order.Back())
	}
	return entry, true
}

func (c *userIDCache) complete(entry *userIDEntry, id string) {
	c.mu.Lock()
	entry.id = id
	entry.expires = c.now().Add(c.ttl)
	c.mu.Unlock()
	close(entry.done)
}

func (c *userIDCache) remove(element *list.Element) {
	c.order.Remove(element)
	delete(c.entries, element.Value.(*userIDEntry).key)
}
