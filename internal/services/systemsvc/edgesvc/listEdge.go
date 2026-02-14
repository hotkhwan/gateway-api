// internal/services/systemsvc/edgesvc/list.go
package edgesvc

import (
	"context"
	"strings"
	"time"

	"klynx/config"
	"klynx/internal/logger"
	"klynx/models/systemmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ListQuery struct {
	Q       string
	Type    string
	Page    int
	PerPage int
}

func ListEdges(ctx context.Context, q ListQuery) ([]systemmod.EdgeListItem, int64, error) {
	log := logger.FromCtx(ctx, "edgesvc", "ListEdges")

	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PerPage <= 0 {
		q.PerPage = 20
	}
	if q.PerPage > 200 {
		q.PerPage = 200
	}

	filter := bson.M{
		"isDeleted": bson.M{"$ne": true},
	}

	if strings.TrimSpace(q.Type) != "" {
		et, err := normalizeEdgeType(q.Type)
		if err != nil {
			return nil, 0, err
		}
		filter["type"] = et
	}

	if s := strings.TrimSpace(q.Q); s != "" {
		rx := bson.M{"$regex": s, "$options": "i"}
		filter["$or"] = []bson.M{
			{"username": rx},
			{"url": rx},
			{"type": rx},
		}
	}

	skip := int64((q.Page - 1) * q.PerPage)
	limit := int64(q.PerPage)

	coll := config.DB.Collection(systemEdgeColl)

	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		log.Error().Err(err).Msg("count edges failed")
		return nil, 0, err
	}

	opt := options.Find().
		SetSort(bson.D{{Key: "updatedAt", Value: -1}}).
		SetSkip(skip).
		SetLimit(limit).
		SetProjection(bson.M{
			"passEnc":      0,
			"apiSecretEnc": 0,
		})

	cur, err := coll.Find(ctx, filter, opt)
	if err != nil {
		log.Error().Err(err).Msg("find edges failed")
		return nil, 0, err
	}
	defer cur.Close(ctx)

	items := make([]systemmod.EdgeListItem, 0, q.PerPage)
	for cur.Next(ctx) {
		var d struct {
			ID        primitive.ObjectID `bson:"_id"`
			Type      systemmod.EdgeType `bson:"type"`
			Username  string             `bson:"username"`
			Name      string             `bson:"name"`
			URL       string             `bson:"url"`
			TLS       bool               `bson:"tls"`
			APIKey    any                `bson:"apiKey,omitempty"`
			CreatedAt time.Time          `bson:"createdAt,omitempty"`
			UpdatedAt time.Time          `bson:"updatedAt,omitempty"`
		}
		if err := cur.Decode(&d); err != nil {
			log.Error().Err(err).Msg("decode edge failed")
			return nil, 0, err
		}
		items = append(items, systemmod.EdgeListItem{
			ID:        d.ID.Hex(),
			Type:      d.Type,
			Username:  d.Username,
			Name:      d.Name,
			URL:       d.URL,
			TLS:       d.TLS,
			APIKey:    d.APIKey,
			CreatedAt: d.CreatedAt,
			UpdatedAt: d.UpdatedAt,
		})
	}
	if err := cur.Err(); err != nil {
		log.Error().Err(err).Msg("cursor error")
		return nil, 0, err
	}

	return items, total, nil
}
