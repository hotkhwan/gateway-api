// internal/services/authzsvc/tupleFactory.go
package authzsvc

import (
	"context"
	"strings"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
)

// func normalizeUserId(userId string) string {
// 	userId = strings.TrimSpace(userId)
// 	if userId == "" {
// 		return ""
// 	}
// 	if strings.HasPrefix(userId, "user:") {
// 		return userId
// 	}
// 	return "user:" + userId
// }

func maskId(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 8 {
		return v
	}
	return v[:4] + "..." + v[len(v)-4:]
}

// TupleFactoryOrgBootstrap: org-only (no orgUnit)
// NEW: Creator becomes owner + member (NOT admin)
// Owner has full management rights via schema permission: manage = owner or admin
func TupleFactoryOrgBootstrap(orgId, userId string) []map[string]interface{} {
	orgId = strings.TrimSpace(orgId)
	userId = strings.TrimSpace(userId)

	subjectUserId := userId
	// subjectUserId := normalizeUserId(userId)

	// ✅ never pass nil context
	log := logger.FromCtx(context.TODO(), "authzsvc", "TupleFactoryOrgBootstrap")

	log.Info().
		Str("orgId", orgId).
		Str("userId_raw", maskId(userId)).
		Str("userId_normalized", maskId(subjectUserId)).
		Str("schemaVersion", strings.TrimSpace(config.CurrentSchemaVersion)).
		Msg("🔧 generating org bootstrap tuples")

	// NEW: Creator is owner + member (NOT admin)
	// Owner relation gives full management rights via schema
	tuples := []map[string]interface{}{
		{
			"entity":   map[string]string{"type": "organization", "id": orgId},
			"relation": "owner", // NEW: set creator as owner ONLY (not admin)
			"subject":  map[string]string{"type": "user", "id": subjectUserId},
		},
		{
			"entity":   map[string]string{"type": "organization", "id": orgId},
			"relation": "member",
			"subject":  map[string]string{"type": "user", "id": subjectUserId},
		},
	}

	log.Info().
		Str("orgId", orgId).
		Int("tupleCount", len(tuples)).
		Msg("✅ org bootstrap tuples generated")

	return tuples
}
