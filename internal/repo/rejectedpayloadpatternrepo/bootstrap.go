// internal/repo/rejectedpayloadpatternrepo/bootstrap.go
package rejectedpayloadpatternrepo

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return ensureIndexes(ctx)
	})
}

func ensureIndexes(ctx context.Context) error {
	return stomongo.CreateIndexes(ctx, col, []mongo.IndexModel{
		{
			// Unique per org+sourceFamily+fingerprint — core dedup key
			Keys:    bson.D{{Key: "orgId", Value: 1}, {Key: "sourceFamily", Value: 1}, {Key: "fingerprint", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			// Lookup by patternId
			Keys:    bson.D{{Key: "orgId", Value: 1}, {Key: "patternId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			// List patterns by org sorted by createdAt desc
			Keys: bson.D{{Key: "orgId", Value: 1}, {Key: "createdAt", Value: -1}},
		},
	})
}
