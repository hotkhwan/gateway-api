// internal/repo/sourceprofilerepo/bootstrap.go
package sourceprofilerepo

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		if err := NewSourceProfileRepo().EnsureIndexes(ctx); err != nil {
			return err
		}
		return seedDefaultProfiles(ctx)
	})
}

// defaultProfiles defines the baseline source profiles seeded on startup.
// Uses $setOnInsert so operator overrides are never overwritten.
var defaultProfiles = []ingestmod.SourceProfile{
	{SourceFamily: "AIBOX", DisplayName: "AIBOX Edge AI", Mode: "active"},
	{SourceFamily: "PVS", DisplayName: "PVS Security", Mode: "active"},
	{SourceFamily: "dahua", DisplayName: "Dahua", Mode: "active"},
	{SourceFamily: "hikvision", DisplayName: "Hikvision", Mode: "comingSoon"},
	{SourceFamily: "mock", DisplayName: "Mock (testing)", Mode: "mock"},
}

// seedDefaultProfiles upserts baseline source profiles.
// $setOnInsert ensures existing profiles (modified by operators) are never overwritten.
func seedDefaultProfiles(ctx context.Context) error {
	now := time.Now().UTC()
	for _, p := range defaultProfiles {
		// stomongo.UpsertByFilter auto-adds updatedAt via nowSet(),
		// so we must NOT include updatedAt in $setOnInsert to avoid conflict.
		_, err := stomongo.UpsertByFilter(
			ctx,
			colSourceProfiles,
			bson.M{"sourceFamily": p.SourceFamily},
			bson.M{}, // no $set — don't overwrite anything on existing docs
			bson.M{
				"sourceFamily": p.SourceFamily,
				"displayName":  p.DisplayName,
				"mode":         p.Mode,
				"createdAt":    now,
			},
		)
		if err != nil {
			return err
		}
	}
	if err := reconcileStaleComingSoon(ctx); err != nil {
		return err
	}

	log.Info().Int("count", len(defaultProfiles)).Msg("[SourceProfile] default profiles seeded")
	return nil
}

// reconcileStaleComingSoon promotes existing profiles whose shipped default is
// now "active" but which still carry a stale "comingSoon" mode from an older
// seed (e.g. dahua, seeded comingSoon before its integration shipped).
//
// seedDefaultProfiles uses $setOnInsert and therefore cannot fix an
// already-seeded doc — so just deploying the new binary would otherwise leave
// dahua stuck on comingSoon. This forward-migration makes "deploy the new
// version" sufficient: it is idempotent (once promoted, no comingSoon docs
// match) and only touches docs at the old comingSoon default — any other
// operator value (active/mock) is left untouched.
func reconcileStaleComingSoon(ctx context.Context) error {
	for _, p := range defaultProfiles {
		if p.Mode != "active" {
			continue // families still shipped as comingSoon (e.g. hikvision) stay as-is
		}
		res, err := stomongo.UpdateMany(
			ctx,
			colSourceProfiles,
			bson.M{"sourceFamily": p.SourceFamily, "mode": "comingSoon"},
			bson.M{"mode": "active"}, // nowSet() stamps updatedAt
		)
		if err != nil {
			return err
		}
		if res != nil && res.ModifiedCount > 0 {
			log.Info().
				Str("sourceFamily", p.SourceFamily).
				Int64("promoted", res.ModifiedCount).
				Msg("[SourceProfile] promoted stale comingSoon -> active")
		}
	}
	return nil
}
