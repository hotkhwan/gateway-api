// internal/repo/unknownpayloadreviewrepo/repo.go
package unknownpayloadreviewrepo

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const col = "unknown_payload_reviews"

var ErrNotFound = errors.New("unknown payload review not found")

type UnknownPayloadReviewRepo struct{}

func NewUnknownPayloadReviewRepo() *UnknownPayloadReviewRepo {
	return &UnknownPayloadReviewRepo{}
}

// Upsert creates a new review or increments seenCount + updates lastSeenAt if one exists.
// Returns the reviewId (new or existing).
func (r *UnknownPayloadReviewRepo) Upsert(
	ctx context.Context,
	reviewId, orgId, sourceFamily, fingerprint string,
	samplePayload map[string]any,
	candidateSuggestionIds []string,
) (string, error) {
	if candidateSuggestionIds == nil {
		candidateSuggestionIds = []string{}
	}
	now := time.Now().UTC()
	filter := bson.M{
		"workspaceId":  orgId,
		"sourceFamily": sourceFamily,
		"fingerprint":  fingerprint,
	}
	update := bson.M{
		"$set": bson.M{
			"lastSeenAt":    now,
			"updatedAt":     now,
			"samplePayload": samplePayload,
		},
		"$inc": bson.M{"seenCount": 1},
		"$setOnInsert": bson.M{
			"reviewId":               reviewId,
			"workspaceId":            orgId,
			"sourceFamily":           sourceFamily,
			"fingerprint":            fingerprint,
			"status":                 "pending",
			"candidateSuggestionIds": candidateSuggestionIds,
			"firstSeenAt":            now,
			"createdAt":              now,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err := config.DB.Collection(col).UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return "", err
	}

	// Return reviewId: fetch it (it's either the one we set or the existing one)
	var result ingestmod.UnknownPayloadReview
	if err := stomongo.FindOne(ctx, col, filter, &result); err != nil {
		return reviewId, nil // fallback to the one we generated
	}
	return result.ReviewId, nil
}

func (r *UnknownPayloadReviewRepo) FindById(ctx context.Context, orgId, reviewId string) (*ingestmod.UnknownPayloadReview, error) {
	var result ingestmod.UnknownPayloadReview
	err := stomongo.FindOne(ctx, col, bson.M{"workspaceId": orgId, "reviewId": reviewId}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (r *UnknownPayloadReviewRepo) List(
	ctx context.Context,
	orgId string,
	page, perPage int,
) ([]*ingestmod.UnknownPayloadReview, *gmod.Pagination, error) {
	filter := bson.M{"workspaceId": orgId, "status": "pending"}
	sort := bson.D{{Key: "createdAt", Value: -1}}

	var result []*ingestmod.UnknownPayloadReview
	pagination, err := stomongo.FindWithPagination(ctx, col, filter, sort, page, perPage, &result)
	if err != nil {
		return nil, nil, err
	}
	return result, &pagination, nil
}

// MarkRejected sets status to "rejected".
func (r *UnknownPayloadReviewRepo) MarkRejected(ctx context.Context, orgId, reviewId string) error {
	res, err := stomongo.UpdateOne(ctx, col, bson.M{"workspaceId": orgId, "reviewId": reviewId}, bson.M{"status": "rejected"})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete hard-deletes a review (called when template is created successfully).
func (r *UnknownPayloadReviewRepo) Delete(ctx context.Context, orgId, reviewId string) error {
	res, err := stomongo.DeleteOne(ctx, col, bson.M{"workspaceId": orgId, "reviewId": reviewId})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *UnknownPayloadReviewRepo) EnsureIndexes(ctx context.Context) error {
	// Migrate legacy orgId field → workspaceId for documents written before the rename.
	_, _ = stomongo.UpdateManyPipeline(ctx, col,
		bson.M{"orgId": bson.M{"$exists": true}},
		mongo.Pipeline{
			bson.D{{Key: "$set", Value: bson.M{"workspaceId": "$orgId"}}},
			bson.D{{Key: "$unset", Value: "orgId"}},
		},
	)

	return stomongo.CreateIndexes(ctx, col, []mongo.IndexModel{
		{
			// Unique per org+sourceFamily+fingerprint — prevents duplicate reviews
			Keys:    bson.D{{Key: "workspaceId", Value: 1}, {Key: "sourceFamily", Value: 1}, {Key: "fingerprint", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			// Lookup by reviewId
			Keys:    bson.D{{Key: "workspaceId", Value: 1}, {Key: "reviewId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			// List pending reviews sorted by createdAt
			Keys: bson.D{{Key: "workspaceId", Value: 1}, {Key: "status", Value: 1}, {Key: "createdAt", Value: -1}},
		},
	})
}
