// internal/repo/templatereviewrepo/repo.go
package templatereviewrepo

import (
	"context"
	"errors"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const colTemplateReviews = "template_reviews"

var ErrNotFound = errors.New("template review not found")

type TemplateReviewRepo struct{}

func NewTemplateReviewRepo() *TemplateReviewRepo {
	return &TemplateReviewRepo{}
}

func (r *TemplateReviewRepo) Insert(ctx context.Context, rev *ingestmod.TemplateReview) error {
	_, err := stomongo.InsertOne(ctx, colTemplateReviews, rev)
	return err
}

func (r *TemplateReviewRepo) FindById(ctx context.Context, tenantId, orgId, reviewId string) (*ingestmod.TemplateReview, error) {
	var result ingestmod.TemplateReview
	err := stomongo.FindOne(ctx, colTemplateReviews, bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"reviewId": reviewId,
	}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (r *TemplateReviewRepo) List(
	ctx context.Context,
	tenantId, orgId string,
	page, perPage int,
) ([]*ingestmod.TemplateReview, *gmod.Pagination, error) {
	filter := bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"status":   "pending",
	}
	sort := bson.D{{Key: "createdAt", Value: -1}}

	var result []*ingestmod.TemplateReview
	pagination, err := stomongo.FindWithPagination(ctx, colTemplateReviews, filter, sort, page, perPage, &result)
	if err != nil {
		return nil, nil, err
	}
	return result, &pagination, nil
}

func (r *TemplateReviewRepo) Archive(ctx context.Context, tenantId, orgId, reviewId string) error {
	res, err := stomongo.UpdateOne(ctx, colTemplateReviews, bson.M{
		"tenantId": tenantId,
		"orgId":    orgId,
		"reviewId": reviewId,
	}, bson.M{"status": "archived"})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *TemplateReviewRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.CreateIndexes(ctx, colTemplateReviews, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}, {Key: "reviewId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}, {Key: "sourceFamily", Value: 1}, {Key: "fingerprint", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}, {Key: "status", Value: 1}, {Key: "createdAt", Value: -1}},
		},
	})
}
