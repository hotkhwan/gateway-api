// internal/services/authzsvc/handlemessage.go
package authzsvc

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

func HandleRelationshipEvent(ctx context.Context, evt authzmod.RelationshipEvent) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.HandleRelationshipEvent",
		"authzsvc", "HandleRelationshipEvent",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	payload, _ := json.Marshal(evt)
	log.Debug().
		RawJSON("event", payload).
		Msg("📥 Received relationship event (REST)")

	// ctx := context.Background()
	switch evt.Type {
	case "group_member_added":
		if err := GrantUserToGroup(ctx, evt.GroupID, evt.UserID); err != nil {
			log.Error().
				Err(err).
				Str("groupId", evt.GroupID).
				Str("userId", evt.UserID).
				Msg("❌ Failed to add group member (REST)")
		}
	case "resource_relocated":
		// ✅ REST Version ไม่ต้องส่ง ctx และ schemaVersion
		if err := EventUpdateResourceRelationship(ctx, evt.ResourceID, evt.NewGroupID); err != nil {
			log.Error().
				Err(err).
				Str("resourceId", evt.ResourceID).
				Str("newGroupId", evt.NewGroupID).
				Msg("❌ Failed to relocate resource (REST)")
		}
	}
}
