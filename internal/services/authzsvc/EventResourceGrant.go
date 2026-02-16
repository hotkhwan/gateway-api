// internal/services/authzsvc/resourceGrant.go
package authzsvc

import (
	"context"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"time"
)

func EventUpdateResourceRelationship(ctx context.Context, ResourceID, groupID string) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.EventUpdateResourceRelationship",
		"authzsvc", "EventUpdateResourceRelationship",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	log.Debug().
		Str("resourceId", ResourceID).
		Str("groupID", groupID).
		Msg("📌 Event-drivent granting resource via Permify (REST)")
	tuples := []map[string]interface{}{
		{
			"entity":   map[string]string{"type": "sensor_device", "id": ResourceID},
			"relation": "located_in_group",
			"subject":  map[string]string{"type": "group", "id": groupID},
		},
	}
	return writeDataREST(ctx, tuples, nil)
}
