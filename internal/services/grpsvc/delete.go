// internal/services/grpsvc/delete.go
package grpsvc

import (
	"context"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func DeleteGroup(ctx context.Context, id string, cascade string) error {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/grpsvc", "grpsvc.DeleteGroup", "grpsvc", "DeleteGroup")
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("groups")

	filter, err := idFilter(id)
	if err != nil {
		return gmod.Errorf("GROUP_NOT_FOUND", "invalid id")
	}

	// มีจริงไหม
	var target bson.M
	if err := coll.FindOne(ctx, filter).Decode(&target); err != nil {
		return gmod.Errorf("GROUP_NOT_FOUND", "group not found")
	}

	// ตรวจลูก
	childCount, err := coll.CountDocuments(ctx, bson.M{"parentId": id})
	if err != nil {
		return err
	}

	switch cascade {
	case "": // ไม่ระบุ
		if childCount > 0 {
			return gmod.Errorf("HAS_CHILDREN", "group has children")
		}
	case "detach": // เด็ก ๆ ถูกเลื่อนขึ้น root
		if childCount > 0 {
			if _, err := coll.UpdateMany(ctx, bson.M{"parentId": id}, bson.M{"$set": bson.M{"parentId": nil}}); err != nil {
				return err
			}
		}
	case "delete":
		// ไม่รองรับ (กันพลาด)
		return gmod.Errorf("UNSUPPORTED_CASCADE", "cascade=delete is not supported")
	default:
		return gmod.Errorf("UNSUPPORTED_CASCADE", "unsupported cascade option")
	}

	if _, err := coll.DeleteOne(ctx, filter); err != nil {
		return err
	}

	log.Info().Str("id", id).Str("cascade", cascade).Msg("✅ Group deleted")
	return nil
}
