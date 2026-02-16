// internal/services/devsync/svmsSyncEdge.go
package devsync

import (
	"context"
	"fmt"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/devsyncmod"
	"github.com/hotkhwan/gateway-api/models/systemmod"
	"github.com/hotkhwan/gateway-api/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func SyncDevicesAndChannelsByEdgeIDSVMS(ctx context.Context, edgeID string) (*devsyncmod.SyncResult, error) {
	oid, err := primitive.ObjectIDFromHex(edgeID)
	if err != nil {
		return nil, fmt.Errorf("invalid id")
	}

	coll := config.MongoClient.Database("klynx").Collection("system_edges")

	var edge systemmod.EdgeConfig
	if err := coll.FindOne(ctx, bson.M{
		"_id":  oid,
		"type": "svms",
	}).Decode(&edge); err != nil {
		return nil, ErrEdgeNotFound
	}

	passwordPlain, err := utils.DecryptFromKeyringJSON(edge.PassEnc)
	if err != nil {
		return nil, err
	}

	total, inserted, err := SyncFromSVMS(ctx, SyncConfig{
		BaseURL:  edge.URL,
		User:     edge.Username,
		Pass:     passwordPlain,
		PageSize: 200,
	}, config.MongoClient.Database("klynx"))
	if err != nil {
		return nil, err
	}

	// SVMS sync โค้ดเดิมนับ upsert เฉพาะ insert ใหม่
	// เพื่อให้ model สมเหตุสมผล: inserted = upserts, updated = total - upserts (โดยประมาณ)
	updated := total - inserted
	if updated < 0 {
		updated = 0
	}

	return &devsyncmod.SyncResult{
		Devices:  1,
		Channels: total,
		Inserted: inserted,
		Updated:  updated,
		PerDeviceCounts: map[int64]int{
			1: total,
		},
	}, nil
}
