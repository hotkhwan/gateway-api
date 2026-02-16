// internal/services/authzsvc/bulkPermission.go
package authzsvc

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// helper แปลง CRUD → permify permission name (ตาม schema resource)
func crudToPermission(act string) string {
	switch act {
	case "c", "u":
		return "edit"
	case "r":
		return "view"
	case "d":
		return "delete"
	default:
		// ถ้าเป็น custom action เช่น "admin_access" ก็คืนค่าเดิมให้ไป match permission ตรง ๆ
		return act
	}
}

// GetGlobalPermissionsForSubject ใช้ Permify batch API เช็คทีเดียวทั้ง list
//
// resources map[resourceID]struct{Type, Acts}
//   - Type: ถ้าเว้นว่าง → default "resource"
//   - Acts: CRUD / action เช่น ["r","u","d"] หรือ ["view","edit"]
//
// return: map[resourceID][]actAllowed  (ใช้ค่า act เดิมที่ส่งเข้ามา)
func GetGlobalPermissionsForSubject(
	ctx context.Context,
	subjectType string,
	subjectID string,
	resources map[string]struct {
		Type string
		Acts []string
	},
) (map[string][]string, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.GetGlobalPermissionsForSubject",
		"authzsvc", "GetGlobalPermissionsForSubject",
	)
	defer end()

	// ให้ทั้ง batch มี timeout รวม 5s
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log.Info().
		Str("subjectType", subjectType).
		Str("subjectId", subjectID).
		Int("resourceCount", len(resources)).
		Msg("🔄 Starting bulk permission checks (batch API)")

	// 1) เตรียม input สำหรับ batch call
	inputs := make([]authzmod.PermissionCheckInput, 0, len(resources)*3)
	for resID, cfg := range resources {
		resourceType := cfg.Type
		if resourceType == "" {
			resourceType = "resource"
		}
		for _, act := range cfg.Acts {
			perm := crudToPermission(act)
			inputs = append(inputs, authzmod.PermissionCheckInput{
				EntityType:  resourceType,
				EntityID:    resID,
				Permission:  perm,
				SubjectType: subjectType,
				SubjectID:   subjectID,
			})
		}
	}

	if len(inputs) == 0 {
		return map[string][]string{}, nil
	}

	// 2) ยิง /permissions/check/batch ครั้งเดียว
	matrix, err := CheckPermissionsBatchMatrix(ctx, inputs)
	if err != nil {
		log.Error().Err(err).Msg("❌ Batch permission check failed")
		return nil, err
	}

	// 3) map กลับเป็น CRUD ที่ allowed
	results := make(map[string][]string, len(resources))
	for resID, cfg := range resources {
		for _, act := range cfg.Acts {
			perm := crudToPermission(act)
			if permsForEntity, ok := matrix[resID]; ok {
				if allowed, ok2 := permsForEntity[perm]; ok2 && allowed {
					results[resID] = append(results[resID], act)
				}
			}
		}
	}

	log.Debug().
		Str("subjectType", subjectType).
		Str("subjectId", subjectID).
		Interface("results", results).
		Msg("✅ Bulk permission checked (batch API)")

	return results, nil
}

// ตัวเก่า สำหรับ compatibility เดิม (subject=user)
// ตอนนี้ delegate ไป GetGlobalPermissionsForSubject
func GetUserGlobalPermissions(
	ctx context.Context,
	userID string,
	resources map[string]struct {
		Type string
		Acts []string
	},
) (map[string][]string, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.GetUserGlobalPermissions",
		"authzsvc", "GetUserGlobalPermissions",
	)
	defer end()

	log.Info().
		Str("userId", userID).
		Int("resourceCount", len(resources)).
		Msg("🔄 Starting bulk permission checks for user (batch API)")

	return GetGlobalPermissionsForSubject(ctx, "user", userID, resources)
}
