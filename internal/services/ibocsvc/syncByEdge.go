// internal/services/ibocsvc/syncByEdge.go
package ibocsvc

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"klynx/config"
	"klynx/internal/gateways/atagw"
	"klynx/models/aimodel"
	"klynx/models/systemmod"
	"klynx/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// var ErrEdgeNotFound = fmt.Errorf("edge not found")
type SyncResult struct {
	Devices         int              `json:"devices"`
	Channels        int              `json:"channels"`
	Inserted        int              `json:"inserted"`
	Updated         int              `json:"updated"`
	PerDeviceCounts map[int64]int    `json:"perDeviceCounts"`
	ChannelsSample  []map[string]any `json:"channelsSample,omitempty"`
}

func SyncDevicesAndChannelsByEdgeID(ctx context.Context, edgeID string) (*SyncResult, error) {
	oid, err := primitive.ObjectIDFromHex(edgeID)
	if err != nil {
		return nil, fmt.Errorf("invalid edgeId")
	}

	// ✅ correct collection
	coll := config.MongoClient.Database("klynx").Collection("system_edges")

	var edge systemmod.EdgeConfig
	if err := coll.FindOne(ctx, bson.M{
		"_id":  oid,
		"type": "ata",
	}).Decode(&edge); err != nil {
		return nil, ErrEdgeNotFound
	}

	// 🔐 decrypt password
	passwordPlain, err := utils.DecryptFromKeyringJSON(edge.PassEnc)
	if err != nil {
		return nil, err
	}

	// 🔑 ATA requires triple-hash (same as sync default)
	passwordHash := tripleHashPassword(edge.Username, passwordPlain)

	// 🌐 create ATA gateway client
	gw := &atagw.Client{
		BaseURL: edge.URL,
	}

	// 🔐 login
	token, err := gw.Login(ctx, edge.Username, passwordHash)
	if err != nil {
		return nil, err
	}

	cameraColl := config.MongoClient.Database("klynx").Collection("camera")

	var (
		inserted int
		updated  int
		totalCh  int
	)

	// ATA ไม่มี list devices endpoint ใน gateway นี้ → deviceId = 1 (ตาม behavior เดิม)
	deviceID := int64(1)

	// fetch channels
	channels, err := gw.GetChannels(ctx, token, int(deviceID), 1, 200)
	if err != nil {
		return nil, err
	}

	totalCh = len(channels)

	for _, ch := range channels {
		// 🧱 reuse mapping เดิม 100%
		doc, err := buildCameraDocFromChannel(
			&Client{cfg: Config{
				Username: edge.Username,
			}},
			// fake device (ATA edge = 1 device)
			aimodel.ATADevice{
				ID:   deviceID,
				Name: ch.Device.Name,
				SN:   ch.SN,
				IP:   ch.Device.IP,
			},
			ch,
		)
		if err != nil {
			continue
		}

		update := bson.M{
			"$set":         doc,
			"$setOnInsert": bson.M{"createdAt": time.Now().UTC()},
		}

		// ❗ preserve lat/long ถ้า upstream ว่าง
		if ch.Latitude != "" && ch.Longitude != "" {
			if lat, err := strconv.ParseFloat(ch.Latitude, 64); err == nil {
				update["$set"].(map[string]any)["lat"] = lat
			}
			if lon, err := strconv.ParseFloat(ch.Longitude, 64); err == nil {
				update["$set"].(map[string]any)["long"] = lon
			}
		}

		res, err := cameraColl.UpdateOne(
			ctx,
			bson.M{
				"ata.deviceId":  deviceID,
				"ata.channelId": ch.ID,
			},
			update,
			options.Update().SetUpsert(true),
		)
		if err != nil {
			return nil, err
		}

		if res.MatchedCount == 0 && res.UpsertedCount == 1 {
			inserted++
		} else {
			updated++
		}
	}

	return &SyncResult{
		Devices:         1,
		Channels:        totalCh,
		Inserted:        inserted,
		Updated:         updated,
		PerDeviceCounts: map[int64]int{deviceID: totalCh},
	}, nil
}
