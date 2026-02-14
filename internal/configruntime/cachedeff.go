// internal/configruntime/cachedeff.go
package configruntime

import (
	"context"
	"sync/atomic"
	"time"

	"klynx/internal/repo/optionsrepo"
	"klynx/models/systemmod"
)

type CachedEffectiveLoader struct {
	repo optionsrepo.Repo
	ttl  time.Duration

	// cache
	lastNS atomic.Int64 // last refresh in UnixNano
	val    atomic.Value // *systemmod.EffectiveConfig
}

// NewCachedEffectiveLoader สร้าง loader ที่จะ cache ผลลัพธ์ตาม TTL
func NewCachedEffectiveLoader(repo optionsrepo.Repo, ttl time.Duration) *CachedEffectiveLoader {
	c := &CachedEffectiveLoader{
		repo: repo,
		ttl:  ttl,
	}
	return c
}

// Get คืน EffectiveConfig ล่าสุด ถ้า cache ยังไม่หมดอายุจะคืนค่าจาก cache
func (c *CachedEffectiveLoader) Get(ctx context.Context) (*systemmod.EffectiveConfig, error) {
	now := time.Now()
	last := time.Unix(0, c.lastNS.Load())

	// คืน cache ถ้ายังไม่หมดอายุ
	if v := c.val.Load(); v != nil && now.Sub(last) < c.ttl {
		if eff, ok := v.(*systemmod.EffectiveConfig); ok {
			return eff, nil
		}
	}

	// โหลดใหม่จาก repo
	eff, err := c.repo.LoadEffective(ctx)
	if err != nil {
		// ถ้าโหลดพลาดและมีค่าเก่า ให้คืนค่าเก่าเพื่อความทนทาน
		if v := c.val.Load(); v != nil {
			if old, ok := v.(*systemmod.EffectiveConfig); ok {
				return old, nil
			}
		}
		return nil, err
	}

	c.val.Store(eff)
	c.lastNS.Store(now.UnixNano())
	return eff, nil
}
