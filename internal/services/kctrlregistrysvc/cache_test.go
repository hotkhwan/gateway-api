package kctrlregistrysvc

import (
	"testing"
	"time"

	"github.com/hotkhwan/gateway-api/models/kctrlmod"
)

func TestRegistryCache_PutGet(t *testing.T) {
	c := newRegistryCache(4, 1*time.Hour)
	row := &kctrlmod.KctrlRegistry{HwId: "h1", Approved: true}
	c.Put("h1", row)
	got, ok := c.Get("h1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != row {
		t.Errorf("got %p, want %p", got, row)
	}
}

func TestRegistryCache_NegativeCache(t *testing.T) {
	// Caching nil ("not found") is supported per contract §6 so unknown
	// devices don't pay the Mongo cost on every message.
	c := newRegistryCache(4, 1*time.Hour)
	c.Put("h1", nil)
	got, ok := c.Get("h1")
	if !ok {
		t.Fatal("expected negative-cache hit")
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestRegistryCache_TTLExpiry(t *testing.T) {
	c := newRegistryCache(4, 5*time.Second)
	now := time.Unix(1000, 0)
	c.clock = func() time.Time { return now }
	c.Put("h1", &kctrlmod.KctrlRegistry{HwId: "h1"})

	now = now.Add(5*time.Second + 1*time.Nanosecond)
	if _, ok := c.Get("h1"); ok {
		t.Error("expected miss after TTL expiry")
	}
	if c.Len() != 0 {
		t.Error("expected entry evicted on lazy expire")
	}
}

func TestRegistryCache_LRUEviction(t *testing.T) {
	// Capacity 2 → inserting 3 keys must drop the oldest.
	c := newRegistryCache(2, 1*time.Hour)
	c.Put("a", &kctrlmod.KctrlRegistry{HwId: "a"})
	c.Put("b", &kctrlmod.KctrlRegistry{HwId: "b"})
	// touch "a" so it's the most recent
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a missing")
	}
	c.Put("c", &kctrlmod.KctrlRegistry{HwId: "c"})

	if _, ok := c.Get("b"); ok {
		t.Error("expected b to be evicted (LRU tail)")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("expected a to survive (recently used)")
	}
	if _, ok := c.Get("c"); !ok {
		t.Error("expected c to be present (just inserted)")
	}
}

func TestRegistryCache_Invalidate(t *testing.T) {
	c := newRegistryCache(4, 1*time.Hour)
	c.Put("h1", &kctrlmod.KctrlRegistry{HwId: "h1"})
	c.Invalidate("h1")
	if _, ok := c.Get("h1"); ok {
		t.Error("expected miss after invalidate")
	}
	// Invalidate on missing key is safe (no panic, no error)
	c.Invalidate("nonexistent")
}

func TestRegistryCache_RefreshKeepsKey(t *testing.T) {
	c := newRegistryCache(2, 1*time.Hour)
	c.Put("a", &kctrlmod.KctrlRegistry{HwId: "a"})
	c.Put("a", &kctrlmod.KctrlRegistry{HwId: "a", Approved: true})
	got, ok := c.Get("a")
	if !ok || got == nil || !got.Approved {
		t.Errorf("expected refreshed value with Approved=true, got %+v ok=%v", got, ok)
	}
	if c.Len() != 1 {
		t.Errorf("expected len 1 after refresh, got %d", c.Len())
	}
}
