// internal/repo/authzrepo/effectiveTuples.go
package authzrepo

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/models/authzmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	effectiveTuplesCollection = "profile_effective_tuples"
)

type ProfileEffectiveTupleRepo interface {
	// ผูก profileCode เข้ากับ tuple key (add-only, idempotent)
	AttachProfile(ctx context.Context, profileCode string, key authzmod.TupleKey) error

	// ถอด profileCode ออกจาก tuple key
	// - owned == false  → profile นี้ไม่เคยเป็นเจ้าของ tuple นี้
	// - owned == true && len(remainingProfiles) == 0 → ไม่มี profile อื่นแล้ว, caller ควรลบ tuple ใน Permify
	DetachProfile(ctx context.Context, profileCode string, key authzmod.TupleKey) (remainingProfiles []string, owned bool, err error)

	// สำหรับ debug / drift: list tuple keys ทั้งหมดที่ profile นี้เคยแตะ
	ListByProfile(ctx context.Context, profileCode string) ([]authzmod.TupleKey, error)

	GetTuplesByProfile(ctx context.Context, profileCode string) ([]authzmod.ProfileEffectiveTuple, error)

	PullProfileCode(ctx context.Context, id primitive.ObjectID, profileCode string) error
}

type profileEffectiveTupleRepo struct {
	coll *mongo.Collection
}

func NewProfileEffectiveTupleRepo(db *mongo.Database) ProfileEffectiveTupleRepo {
	coll := db.Collection(effectiveTuplesCollection)

	// unique index: ต่อหนึ่ง tuple key มี document เดียว
	_, _ = coll.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.D{
			{Key: "entity_type", Value: 1},
			{Key: "entity_id", Value: 1},
			{Key: "relation", Value: 1},
			{Key: "subject_type", Value: 1},
			{Key: "subject_id", Value: 1},
		},
		Options: options.Index().SetUnique(true).SetName("ux_tuple_key"),
	})

	return &profileEffectiveTupleRepo{coll: coll}
}

func (r *profileEffectiveTupleRepo) AttachProfile(ctx context.Context, profileCode string, key authzmod.TupleKey) error {
	now := time.Now().UTC()

	filter := bson.M{
		"entity_type":  key.EntityType,
		"entity_id":    key.EntityID,
		"relation":     key.Relation,
		"subject_type": key.SubjectType,
		"subject_id":   key.SubjectID,
	}

	update := bson.M{
		"$setOnInsert": bson.M{
			"created_at": now,
		},
		"$set": bson.M{
			"entity_type":  key.EntityType,
			"entity_id":    key.EntityID,
			"relation":     key.Relation,
			"subject_type": key.SubjectType,
			"subject_id":   key.SubjectID,
			"updated_at":   now,
		},
		"$addToSet": bson.M{
			"profile_codes": profileCode,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := r.coll.UpdateOne(ctx, filter, update, opts)
	return err
}

func (r *profileEffectiveTupleRepo) DetachProfile(ctx context.Context, profileCode string, key authzmod.TupleKey) (remaining []string, owned bool, err error) {
	now := time.Now().UTC()

	filter := bson.M{
		"entity_type":  key.EntityType,
		"entity_id":    key.EntityID,
		"relation":     key.Relation,
		"subject_type": key.SubjectType,
		"subject_id":   key.SubjectID,
	}

	update := bson.M{
		"$pull": bson.M{
			"profile_codes": profileCode,
		},
		"$set": bson.M{
			"updated_at": now,
		},
	}

	var doc authzmod.ProfileEffectiveTuple
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	err = r.coll.FindOneAndUpdate(ctx, filter, update, opts).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		// ไม่มี record → profile นี้ไม่เคยเป็นเจ้าของ tuple นี้
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	remaining = doc.ProfileCodes
	owned = true

	// ถ้าไม่มี profile ไหนเหลืออยู่ → ลบ document ทิ้ง
	if len(remaining) == 0 {
		_, _ = r.coll.DeleteOne(ctx, bson.M{"_id": doc.ID})
	}

	return remaining, owned, nil
}

func (r *profileEffectiveTupleRepo) ListByProfile(
	ctx context.Context,
	profileCode string,
) ([]authzmod.TupleKey, error) {
	if profileCode == "" {
		return nil, errors.New("profileCode is required")
	}

	filter := bson.M{
		"profile_codes": profileCode,
	}

	cur, err := r.coll.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var result []authzmod.TupleKey
	for cur.Next(ctx) {
		var doc struct {
			EntityType  string   `bson:"entity_type"`
			EntityID    string   `bson:"entity_id"`
			Relation    string   `bson:"relation"`
			SubjectType string   `bson:"subject_type"`
			SubjectID   string   `bson:"subject_id"`
			Profiles    []string `bson:"profile_codes"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		result = append(result, authzmod.TupleKey{
			EntityType:  doc.EntityType,
			EntityID:    doc.EntityID,
			Relation:    doc.Relation,
			SubjectType: doc.SubjectType,
			SubjectID:   doc.SubjectID,
		})
	}

	return result, cur.Err()
}

// GetTuplesByProfile: ดึง Tuple ที่มี pf_cam1 อยู่ใน list
func (r *profileEffectiveTupleRepo) GetTuplesByProfile(ctx context.Context, profileCode string) ([]authzmod.ProfileEffectiveTuple, error) {
	collection := r.coll.Database().Collection("profile_effective_tuples")

	// ค้นหา Tuple ที่มี profileCode นี้อยู่ในอาเรย์ profile_codes
	filter := bson.M{"profile_codes": profileCode}
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	var results []authzmod.ProfileEffectiveTuple
	err = cursor.All(ctx, &results)
	return results, err
}

// PullProfile: ดึง profileCode ออกจากอาเรย์
func (r *profileEffectiveTupleRepo) PullProfileCode(ctx context.Context, id primitive.ObjectID, profileCode string) error {
	collection := r.coll.Database().Collection("profile_effective_tuples")

	_, err := collection.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$pull": bson.M{"profile_codes": profileCode}},
	)
	return err
}
