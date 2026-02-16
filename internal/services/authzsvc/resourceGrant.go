package authzsvc

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// GrantResource creates relationships + attributes for a resource in Permify (REST).
func GrantResource(ctx context.Context, req authzmod.GrantResourceRequest) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.GrantResource",
		"authzsvc", "GrantResource",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	log.Debug().Str("resourceId", req.ResourceID).Msg("📌 Granting resource via Permify (REST)")

	tuples := buildResourceRelationshipTuples(req)
	attributes := buildResourceAttributes(req)

	if err := writeDataREST(ctx, tuples, attributes); err != nil {
		log.Error().Err(err).Msg("❌ Failed to write resource data to Permify")
		return err
	}

	log.Debug().Msg("✅ Resource granted successfully via Permify")
	return nil
}

func buildResourceRelationshipTuples(req authzmod.GrantResourceRequest) []map[string]interface{} {
	tuples := []map[string]interface{}{}

	add := func(relation, idType, id string) {
		if id != "" {
			tuples = append(tuples, map[string]interface{}{
				"entity":   map[string]string{"type": "resource", "id": req.ResourceID},
				"relation": relation,
				"subject":  map[string]string{"type": idType, "id": id},
			})
		}
	}

	// core relations
	add("assigned_group", "group", req.AssignedGroupID)
	add("assigned_role", "role", req.AssignedRoleID)
	add("owner", "user", req.OwnerUserID)

	for _, editor := range req.EditorUserIDs {
		add("editor", "user", editor)
	}
	for _, viewer := range req.ViewerUserIDs {
		add("viewer", "user", viewer)
	}

	// รวม ExtraTuples ถ้ามี (เช่น กรณีพิเศษ)
	if len(req.ExtraTuples) > 0 {
		tuples = append(tuples, req.ExtraTuples...)
	}

	return tuples
}

func buildResourceAttributes(req authzmod.GrantResourceRequest) []map[string]interface{} {
	attrs := []map[string]interface{}{}

	add := func(attr, val string) {
		if val != "" {
			attrs = append(attrs, map[string]interface{}{
				"entity":    map[string]string{"type": "resource", "id": req.ResourceID},
				"attribute": attr,
				"value": map[string]interface{}{
					"@type": "type.googleapis.com/base.v1.StringValue",
					"data":  val,
				},
			})
		}
	}

	add("type", req.Type)
	add("crud", req.Crud)
	return attrs
}

func GrantUserToGroup(ctx context.Context, groupID, userID string) error {
	return GrantResource(ctx, authzmod.GrantResourceRequest{
		ResourceID: groupID,
		Type:       "group",
		ExtraTuples: []map[string]interface{}{{
			"entity":   map[string]string{"type": "group", "id": groupID},
			"relation": "member",
			"subject":  map[string]string{"type": "user", "id": userID},
		}},
	})
}
