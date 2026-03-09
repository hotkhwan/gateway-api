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
	{SourceFamily: "dahua", DisplayName: "Dahua", Mode: "comingSoon"},
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
	log.Info().Int("count", len(defaultProfiles)).Msg("[SourceProfile] default profiles seeded")
	return nil
}
