// Package clients — catalog_cache.go provides a tiny per-process TTL cache
// for the relatively-static DGC catalogs (asset types, relation types,
// attribute types, statuses, ManagedControl attribute assignments).
//
// These catalogs change rarely (admin-driven schema work), so the
// create-control flow shouldn't re-fetch them on every tool call. The
// cache is hot per chip-binary lifetime; restart clears it.
//
// Lock-during-fetch deliberately serializes concurrent first calls
// (singleflight semantics for free) so a cold cache doesn't fan out
// duplicate page traversals against DGC.

package clients

import (
	"context"
	"net/http"
	"sync"
	"time"
)

const catalogCacheTTL = time.Hour

type catalogCache[T any] struct {
	mu        sync.Mutex
	value     T
	loaded    bool
	expiresAt time.Time
}

func (c *catalogCache[T]) get(
	ctx context.Context,
	client *http.Client,
	fetch func(ctx context.Context, client *http.Client) (T, error),
) (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded && time.Now().Before(c.expiresAt) {
		return c.value, nil
	}
	v, err := fetch(ctx, client)
	if err != nil {
		var zero T
		return zero, err
	}
	c.value = v
	c.loaded = true
	c.expiresAt = time.Now().Add(catalogCacheTTL)
	return v, nil
}