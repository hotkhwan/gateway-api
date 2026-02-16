// internal/services/authzsvc/permissionCheckBatch.go
package authzsvc

import (
	"context"
	"sync"
	"time"

	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// CheckPermissionsBatch ทำ batch แบบ simple: สรุปเป็น map[entityID]bool
// true ถ้ามี "อย่างน้อยหนึ่ง permission" ของ entity นั้นที่ allowed
func CheckPermissionsBatch(ctx context.Context, inputs []authzmod.PermissionCheckInput) (map[string]bool, error) {
	matrix, err := CheckPermissionsBatchMatrix(ctx, inputs)
	if err != nil {
		return nil, err
	}

	out := make(map[string]bool, len(matrix))
	for entityID, perms := range matrix {
		allowed := false
		for _, v := range perms {
			if v {
				allowed = true
				break
			}
		}
		out[entityID] = allowed
	}
	return out, nil
}

// CheckPermissionsBatchMatrix คืนเป็นเมทริกซ์ [entityID][permission] = allowed
// ใช้ fan-out goroutine เรียก PermissionCheck ทีละตัว แทนการเรียก Permify /batch
func CheckPermissionsBatchMatrix(ctx context.Context, inputs []authzmod.PermissionCheckInput) (map[string]map[string]bool, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.CheckPermissionsBatchMatrix",
		"authzsvc", "CheckPermissionsBatchMatrix",
	)
	defer end()

	result := map[string]map[string]bool{}
	if len(inputs) == 0 {
		return result, nil
	}

	// shared timeout ให้ทั้ง batch
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex

	log.Info().
		Int("checks", len(inputs)).
		Msg("🔄 Starting local batch permission checks (fan-out)")

	for _, in := range inputs {
		in := in // copy for goroutine
		wg.Add(1)

		go func() {
			defer wg.Done()

			// ตอนนี้ใช้ depth=50 เหมือน path อื่น ๆ
			allowed, err := PermissionCheck(
				ctx,
				50,
				in.EntityType,
				in.EntityID,
				in.Permission,
				in.SubjectType,
				in.SubjectID,
			)
			if err != nil {
				log.Warn().
					Str("entityType", in.EntityType).
					Str("entityId", in.EntityID).
					Str("permission", in.Permission).
					Str("subjectType", in.SubjectType).
					Str("subjectId", in.SubjectID).
					Err(err).
					Msg("⚠️ Permission check failed in batch")
				return
			}

			mu.Lock()
			defer mu.Unlock()

			if _, ok := result[in.EntityID]; !ok {
				result[in.EntityID] = map[string]bool{}
			}
			result[in.EntityID][in.Permission] = allowed
		}()
	}

	wg.Wait()

	log.Debug().
		Int("entityCount", len(result)).
		Interface("matrix", result).
		Msg("✅ Local batch permission checks completed")

	return result, nil
}
