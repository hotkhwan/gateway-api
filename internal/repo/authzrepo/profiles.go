// internal/repo/authzrepo/profiles.go
package authzrepo

import (
	"context"
	"time"

	"klynx/models/authzmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	profilesCollection = "profiles"
)

// ProfileRepo handles profile persistence operations
type ProfileRepo interface {
	Create(ctx context.Context, profile *authzmod.Profile) error
	Delete(ctx context.Context, code string) error
	FindByCode(ctx context.Context, code string) (*authzmod.Profile, error)
	Update(ctx context.Context, code string, updates bson.M) error
	List(ctx context.Context, filter bson.M, opts ...*options.FindOptions) ([]*authzmod.Profile, error)
	Count(ctx context.Context, filter bson.M) (int64, error)
}

type profileRepo struct {
	coll *mongo.Collection
}

func NewProfileRepo(db *mongo.Database) ProfileRepo {
	return &profileRepo{
		coll: db.Collection(profilesCollection),
	}
}

func (r *profileRepo) Count(ctx context.Context, filter bson.M) (int64, error) {
	return r.coll.CountDocuments(ctx, filter)
}

func (r *profileRepo) Create(ctx context.Context, profile *authzmod.Profile) error {
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	_, err := r.coll.InsertOne(ctx, profile)
	return err
}
func (r *profileRepo) Delete(ctx context.Context, code string) error {
	now := time.Now().UTC()

	objID, err := primitive.ObjectIDFromHex(code)
	if err != nil {
		return err
	}

	var profile authzmod.Profile
	err = r.coll.FindOne(ctx, bson.M{"_id": objID}).Decode(&profile)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return mongo.ErrNoDocuments
		}
		return err
	}

	profile.IsDeleted = true
	profile.UpdatedAt = now

	_, err = r.coll.UpdateOne(
		ctx,
		bson.M{"_id": objID},
		bson.M{"$set": bson.M{
			"isDeleted": profile.IsDeleted,
			"updatedAt": profile.UpdatedAt,
		}},
	)
	return err

}

func (r *profileRepo) FindByCode(ctx context.Context, code string) (*authzmod.Profile, error) {
	var profile authzmod.Profile
	err := r.coll.FindOne(ctx, bson.M{"code": code}).Decode(&profile)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &profile, err
}

func (r *profileRepo) Update(ctx context.Context, code string, updates bson.M) error {
	// sync your BSON timestamp keys with model tags (camelCase)
	updates["updatedAt"] = time.Now().UTC()
	_, err := r.coll.UpdateOne(ctx, bson.M{"code": code}, bson.M{"$set": updates})
	return err
}

func (r *profileRepo) List(ctx context.Context, filter bson.M, opts ...*options.FindOptions) ([]*authzmod.Profile, error) {
	cursor, err := r.coll.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var profiles []*authzmod.Profile
	if err := cursor.All(ctx, &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}
