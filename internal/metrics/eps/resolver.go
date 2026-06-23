// internal/metrics/eps/resolver.go
package eps

import (
	"context"
	"sync"
	"time"
)

// LimitResolver returns the licensed maxEventsPerSecond for a workspace.
// A value <= 0 means unknown/unlimited (the collector then suppresses the
// limit + percent series). Errors are treated as unknown by the collector
// (fail-open) so a backing-store blip never fabricates an "exceeded" reading.
type LimitResolver interface {
	MaxEventsPerSecond(ctx context.Context, workspaceID string) (int, error)
}

// CustomerResolver maps a workspace to its customer label (tenant id).
type CustomerResolver interface {
	CustomerForWorkspace(ctx context.Context, workspaceID string) (string, error)
}

// ttlCache is a tiny per-key TTL cache shared by the cached resolvers. It
// trims the per-scrape backing-store fan-out (one entitlement/workspace read
// per active workspace) down to once per ttl.
type ttlCache[V any] struct {
	ttl   time.Duration
	now   func() int64
	mu    sync.Mutex
	items map[string]cacheItem[V]
}

type cacheItem[V any] struct {
	val V
	exp int64 // unix seconds
}

func newTTLCache[V any](ttl time.Duration) *ttlCache[V] {
	return &ttlCache[V]{
		ttl:   ttl,
		now:   func() int64 { return time.Now().Unix() },
		items: make(map[string]cacheItem[V]),
	}
}

func (c *ttlCache[V]) get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[key]
	if !ok || c.now() > it.exp {
		var zero V
		return zero, false
	}
	return it.val, true
}

func (c *ttlCache[V]) set(key string, val V) {
	c.mu.Lock()
	c.items[key] = cacheItem[V]{val: val, exp: c.now() + int64(c.ttl/time.Second)}
	c.mu.Unlock()
}

// cachedLimitResolver wraps a LimitResolver with a TTL cache. Errors are not
// cached so the next scrape retries the backing store.
type cachedLimitResolver struct {
	inner LimitResolver
	cache *ttlCache[int]
}

// NewCachedLimitResolver wraps inner so repeated lookups within ttl hit memory.
func NewCachedLimitResolver(inner LimitResolver, ttl time.Duration) LimitResolver {
	return &cachedLimitResolver{inner: inner, cache: newTTLCache[int](ttl)}
}

func (r *cachedLimitResolver) MaxEventsPerSecond(ctx context.Context, workspaceID string) (int, error) {
	if v, ok := r.cache.get(workspaceID); ok {
		return v, nil
	}
	v, err := r.inner.MaxEventsPerSecond(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	r.cache.set(workspaceID, v)
	return v, nil
}

// cachedCustomerResolver wraps a CustomerResolver with a TTL cache.
type cachedCustomerResolver struct {
	inner CustomerResolver
	cache *ttlCache[string]
}

// NewCachedCustomerResolver wraps inner so repeated lookups within ttl hit memory.
func NewCachedCustomerResolver(inner CustomerResolver, ttl time.Duration) CustomerResolver {
	return &cachedCustomerResolver{inner: inner, cache: newTTLCache[string](ttl)}
}

func (r *cachedCustomerResolver) CustomerForWorkspace(ctx context.Context, workspaceID string) (string, error) {
	if v, ok := r.cache.get(workspaceID); ok {
		return v, nil
	}
	v, err := r.inner.CustomerForWorkspace(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	r.cache.set(workspaceID, v)
	return v, nil
}
