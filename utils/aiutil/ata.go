// internal/utils/aiutil/ata.go
package aiutil

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CameraMini struct {
	Lat  float64 `bson:"lat"`
	Long float64 `bson:"long"`
	Name any     `bson:"name" json:"name"`
}

// channelId: event.ChannelId
// name: address/deviceName ที่โชว์ เช่น "Lobby_ชั้น1"
func FindCameraLatLong(ctx context.Context, db *mongo.Database, channelId int64, name string) (*CameraMini, error) {
	coll := db.Collection("camera")
	proj := options.FindOne().SetProjection(bson.M{"lat": 1, "long": 1, "name": 1})

	alive := bson.M{"$or": []bson.M{
		{"isDeleted": bson.M{"$exists": false}},
		{"isDeleted": false},
	}}

	// 1) match ด้วย channelId ก่อน
	if channelId != 0 {
		filter := bson.M{
			"brand": "ATA",
			"$and": bson.A{
				alive,
				bson.M{"$or": []bson.M{
					{"channel": channelId},
					{"streamID": channelId},
					{"ata.channelId": channelId},
				}},
			},
		}

		var cam CameraMini
		err := coll.FindOne(ctx, filter, proj).Decode(&cam)
		if err == nil {
			return &cam, nil
		}
		if err != mongo.ErrNoDocuments {
			return nil, err
		}
	}

	// 2) ไม่เจอค่อย match ด้วย name
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}

	filter := bson.M{
		"brand": "ATA",
		"name":  name,
		"$and":  bson.A{alive},
	}

	var cam CameraMini
	err := coll.FindOne(ctx, filter, proj).Decode(&cam)
	if err == nil {
		return &cam, nil
	}
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return nil, err
}
