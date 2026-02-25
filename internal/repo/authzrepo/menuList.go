// internal/repo/authzrepo/menuList.go
package authzrepo

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/authmod"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var ErrMenuListNotFound = errors.New("menu list not found")

// MenuListRepo reads menu options from the shared "options" collection.
// The menu list is cached in-memory (TTL 5 min) since it rarely changes.
type MenuListRepo struct {
	collection string
	mu         sync.RWMutex
	cache      []authmod.MenuOption
	cachedAt   time.Time
	cacheTTL   time.Duration
}

func NewMenuListRepo() *MenuListRepo {
	return &MenuListRepo{
		collection: "options",
		cacheTTL:   5 * time.Minute,
	}
}

// LoadMenuList returns the current list of MenuOption from MongoDB.
func (r *MenuListRepo) LoadMenuList(ctx context.Context) ([]authmod.MenuOption, error) {
	r.mu.RLock()
	if r.cache != nil && time.Since(r.cachedAt) < r.cacheTTL {
		cached := r.cache
		r.mu.RUnlock()
		return cached, nil
	}
	r.mu.RUnlock()

	var doc authmod.MenuListDoc
	err := stomongo.FindOne(ctx, r.collection, bson.M{"_id": "list.klynx"}, &doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrMenuListNotFound
		}
		return nil, err
	}

	r.mu.Lock()
	r.cache = doc.Menu
	r.cachedAt = time.Now()
	r.mu.Unlock()

	return doc.Menu, nil
}

// ValidMenuIDs returns a set of valid menu IDs for fast lookup.
func (r *MenuListRepo) ValidMenuIDs(ctx context.Context) (map[string]bool, error) {
	menus, err := r.LoadMenuList(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(menus))
	for _, m := range menus {
		set[m.ID] = true
	}
	return set, nil
}
