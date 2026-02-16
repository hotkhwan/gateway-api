// internal/adapters/repo/stomongo/common.go
package stomongo

import (
	"time"

	"github.com/hotkhwan/gateway-api/config"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func coll(name string) *mongo.Collection {
	return config.DB.Collection(name)
}

func nowSet(fields bson.M) bson.M {
	set := bson.M{"updatedAt": time.Now().UTC()} // ⬅️ ใช้ UTC
	for k, v := range fields {
		set[k] = v
	}
	return set
}
