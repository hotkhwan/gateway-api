// internal/services/grpsvc/create.go
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

func CreateGroup(ctx context.Context, req grpmod.GroupRequest) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/grpsvc", // tracerName
		"grpsvc.CreateGroup",                     // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"grpsvc", "CreateGroup",
	)
	defer end()

	// ใส่ timeout หลังเริ่ม span เพื่อสืบทอด trace
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	collection := db.Collection("groups")

	group := bson.M{
		"name":        req.Name,
		"parentId":    req.ParentID,
		"description": req.Description,
		"public":      req.Public,
		"icon":        req.Icon,
		"createdAt":   time.Now(),
	}

	_, err := collection.InsertOne(ctx, group)
	if err != nil {
		log.Error().Err(err).Msg("❌ Insert group failed")
		return err
	}

	log.Info().Str("name", req.Name).Msg("✅ Group inserted")
	return nil
}
