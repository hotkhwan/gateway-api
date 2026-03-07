// internal/repo/sourceprofilerepo/repo.go
package sourceprofilerepo

import (
	"context"
	"errors"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const colSourceProfiles = "source_profiles"

var ErrNotFound = errors.New("source profile not found")

type SourceProfileRepo struct{}

func NewSourceProfileRepo() *SourceProfileRepo {
	return &SourceProfileRepo{}
}

func (r *SourceProfileRepo) Insert(ctx context.Context, p *ingestmod.SourceProfile) error {
	_, err := stomongo.InsertOne(ctx, colSourceProfiles, p)
	return err
}

func (r *SourceProfileRepo) FindByFamily(ctx context.Context, sourceFamily string) (*ingestmod.SourceProfile, error) {
	var result ingestmod.SourceProfile
	err := stomongo.FindOne(ctx, colSourceProfiles, bson.M{"sourceFamily": sourceFamily}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (r *SourceProfileRepo) List(ctx context.Context) ([]*ingestmod.SourceProfile, error) {
	var result []*ingestmod.SourceProfile
	opts := options.Find().SetSort(bson.D{{Key: "sourceFamily", Value: 1}})
	err := stomongo.Find(ctx, colSourceProfiles, bson.M{}, opts, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *SourceProfileRepo) Update(ctx context.Context, sourceFamily string, setFields bson.M) error {
	res, err := stomongo.UpdateOne(ctx, colSourceProfiles, bson.M{"sourceFamily": sourceFamily}, setFields)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SourceProfileRepo) Delete(ctx context.Context, sourceFamily string) error {
	res, err := stomongo.DeleteOne(ctx, colSourceProfiles, bson.M{"sourceFamily": sourceFamily})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SourceProfileRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.CreateIndexes(ctx, colSourceProfiles, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "sourceFamily", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	})
}
