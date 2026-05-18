// internal/services/kctrlregistrysvc/cache.go
//
// Per-process LRU cache for kctrl_registry lookups, per
// klynx-api/docs/contracts/kcontrol-gw-managed-registry.md §6.
//
// Strategy: read-through with explicit invalidation on PATCH/DELETE, plus a
// short TTL safety net. Hot read path is the MQTT subscriber (kctrlsubmsg)
// which calls Get(hwId) once per inbound message.
package kctrlregistrysvc

import (
	"container/list"
	"sync"
	"time"

	"github.com/hotkhwan/gateway-api/models/kctrlmod"
)

// cacheEntry holds either a registry row or a negative result (row == nil
// after a "not found" lookup). Both are cached so the hot ENRICH / DROP /
// FORWARD branching in kctrlsubmsg amortises the Mongo round-trip across
// repeated messages from the same hwId.
type cacheEntry struct {
	row       *kctrlmod.KctrlRegistry // nil = negative cache (row not found)
	expiresAt time.Time
	elem      *list.Element // back-pointer into LRU list for O(1) move-to-front
}

// registryCache is a fixed-capacity LRU with per-entry TTL. The capacity is
// small enough (1000 entries by default) that O(N) scans are not a concern —
// we keep an LRU for predictable eviction under bursty traffic.
type registryCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	clock    func() time.Time // injectable for tests
	entries  map[string]*cacheEntry
	order    *list.List // front = most recent
}

func newRegistryCache(capacity int, ttl time.Duration) *registryCache {
	if capacity <= 0 {
		capacity = 1000
	}
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	return &registryCache{
		capacity: capacity,
		ttl:      ttl,
		clock:    time.Now,
		entries:  make(map[string]*cacheEntry, capacity),
		order:    list.New(),
	}
}

// Get returns (row, found). row may be nil even when found=true — that is the
// cached "row not found" result. The hwId is the list element's Value.
//
// Expired entries are evicted lazily on read so the cache cannot serve stale
// data once the TTL has elapsed.
func (c *registryCache) Get(hwId string) (*kctrlmod.KctrlRegistry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[hwId]
	if !ok {
		return nil, false
	}
	if c.clock().After(e.expiresAt) {
		c.evictLocked(hwId)
		return nil, false
	}
	c.order.MoveToFront(e.elem)
	return e.row, true
}

// Put inserts or refreshes a cache entry for hwId. row may be nil to cache a
// negative ("not found") result with the same TTL — kctrlsubmsg uses this to
// avoid a Mongo round-trip on every retained-message storm from an unknown
// device.
func (c *registryCache) Put(hwId string, row *kctrlmod.KctrlRegistry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[hwId]; ok {
		e.row = row
		e.expiresAt = c.clock().Add(c.ttl)
		c.order.MoveToFront(e.elem)
		return
	}
	if c.order.Len() >= c.capacity {
		// Evict the LRU tail.
		if back := c.order.Back(); back != nil {
			oldKey, _ := back.Value.(string)
			c.evictLocked(oldKey)
		}
	}
	elem := c.order.PushFront(hwId)
	c.entries[hwId] = &cacheEntry{
		row:       row,
		expiresAt: c.clock().Add(c.ttl),
		elem:      elem,
	}
}

// Invalidate removes hwId from the cache. Called by Upsert and Delete handlers
// before they return their HTTP response so the originating replica sees the
// fresh state on its next read.
func (c *registryCache) Invalidate(hwId string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictLocked(hwId)
}

// Len returns the current entry count (test helper).
func (c *registryCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// evictLocked is a helper that requires c.mu to be held.
func (c *registryCache) evictLocked(hwId string) {
	e, ok := c.entries[hwId]
	if !ok {
		return
	}
	c.order.Remove(e.elem)
	delete(c.entries, hwId)
}
