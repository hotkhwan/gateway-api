// internal/services/dashsvc/blacklistUpdate.go
package dashsvc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"klynx/internal/repo/stomongo"
	"klynx/models/dashmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func UpdateBlacklistEvent(ctx context.Context, req dashmod.UpdateBlacklistReq) error {
	oid, err := primitive.ObjectIDFromHex(req.ID)
	if err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}

	set := bson.M{}

	if req.Acknowledged != nil {
		set["acknowledged"] = *req.Acknowledged
	}
	if req.IsDeleted != nil {
		set["isDeleted"] = *req.IsDeleted
	}
	if req.Sop != nil {
		set["sop"] = req.Sop
	}

	if len(set) == 0 {
		return errors.New("no fields to update")
	}

	// ✅ อัปเดตเวลาเสมอ
	set["dateTimeUpdate"] = time.Now().UTC()

	_, err = stomongo.UpdateByID(ctx, collectionATAEvents, oid, set)
	return err
}
