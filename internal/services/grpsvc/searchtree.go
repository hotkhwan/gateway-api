package grpsvc

import (
	"context"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/grpmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func SearchTreeByName(ctx context.Context, name string) ([]grpmod.GroupTree, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/grpsvc",
		"groups.SearchTreeByName",
		"grpsvc", "SearchTreeByName",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	log.Info().Str("search", name).Msg("🔍 Searching groups by name")

	coll := config.MongoClient.Database(os.Getenv("MONGO_DB")).Collection("groups")

	cursor, err := coll.Find(ctx, bson.M{
		"name": bson.M{"$regex": name, "$options": "i"},
	})
	if err != nil {
		log.Error().Err(err).Msg("❌ Mongo Find failed")
		return nil, err
	}
	defer cursor.Close(ctx)

	var groups []grpmod.Group
	if err := cursor.All(ctx, &groups); err != nil {
		log.Error().Err(err).Msg("❌ Cursor decode failed")
		return nil, err
	}

	log.Debug().Int("matched", len(groups)).Msg("✅ Group(s) found")

	var trees []grpmod.GroupTree
	for _, g := range groups {
		trees = append(trees, grpmod.GroupTree{
			ID:          g.ID,
			Name:        g.Name,
			Icon:        g.Icon,
			Public:      g.Public,
			Description: g.Description,
		})
	}

	return trees, nil
}
