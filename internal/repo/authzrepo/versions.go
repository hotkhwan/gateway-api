// internal/repo/authzrepo/versions.go
package authzrepo

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/models/authzmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	versionsCollection = "profile_versions"
)

// ProfileVersionRepo handles profile version persistence operations
type ProfileVersionRepo interface {
	FindLatest(ctx context.Context, code string) (*authzmod.ProfileVersion, error)
	Create(ctx context.Context, v *authzmod.ProfileVersion) error
	List(ctx context.Context, code string) ([]*authzmod.ProfileVersion, error)
	UpdateNote(ctx context.Context, code string, version int, note string) error
}

type versionRepo struct {
	coll *mongo.Collection
}

// NewProfileVersionRepo creates a new profile version repository
func NewProfileVersionRepo(db *mongo.Database) ProfileVersionRepo {
	return &versionRepo{
		coll: db.Collection(versionsCollection),
	}
}

func (r *versionRepo) Create(ctx context.Context, version *authzmod.ProfileVersion) error {
	version.CreatedAt = time.Now().UTC()
	_, err := r.coll.InsertOne(ctx, version)
	return err
}

func (r *versionRepo) FindLatest(ctx context.Context, profileCode string) (*authzmod.ProfileVersion, error) {
	opts := options.FindOne().SetSort(bson.D{{Key: "version", Value: -1}})
	var version authzmod.ProfileVersion
	err := r.coll.FindOne(ctx, bson.M{"profileCode": profileCode}, opts).Decode(&version)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &version, err
}

func (r *versionRepo) List(ctx context.Context, profileCode string) ([]*authzmod.ProfileVersion, error) {
	opts := options.Find().SetSort(bson.D{{Key: "version", Value: -1}})
	cursor, err := r.coll.Find(ctx, bson.M{"profileCode": profileCode}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var versions []*authzmod.ProfileVersion
	if err := cursor.All(ctx, &versions); err != nil {
		return nil, err
	}
	return versions, nil
}

func (r *versionRepo) UpdateNote(ctx context.Context, code string, version int, note string) error {
	_, err := r.coll.UpdateOne(
		ctx,
		bson.M{"profileCode": code, "version": version},
		bson.M{"$set": bson.M{"note": note}},
	)
	return err
}
