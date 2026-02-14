// internal/services/authzsvc/factoryClear.go
package authzsvc

import (
	"context"
	"errors"

	"klynx/internal/logger"
)

// ใช้ร่วมกับทั้ง factory-clear และ factory-reset
var ErrPermifyClientNotInit = errors.New("permify client not initialized")

// FactoryResetTenant ลบทุก tuple ทั้ง tenant (ระวังใช้เฉพาะ dev/stage)
// คืน total / deleted / failed เพื่อนำไป log/response
func FactoryResetTenant(ctx context.Context) (total, deleted, failed int, err error) {
	log := logger.WithMeta("authzsvc", "FactoryResetTenant")

	pc := NewPermifyClient()
	if pc == nil {
		err = ErrPermifyClientNotInit
		return
	}

	const pageSize int32 = 100
	token := ""

	for {
		// 🔁 ดึงทีละ page
		tuples, nextToken, e := pc.ReadAllTuplesOnce(ctx, pageSize, token)
		if e != nil {
			err = e
			return
		}

		// ไม่มี tuple เพิ่ม และไม่มี next token แล้ว ⇒ จบ
		if len(tuples) == 0 && nextToken == "" {
			break
		}

		for _, rel := range tuples {
			total++
			tmap := relToMap(rel) // แปลง Relationship → map[string]interface{} ตรงกับ REST payload

			if e := deleteTupleREST(ctx, tmap); e != nil {
				failed++
				log.Warn().
					Err(e).
					Interface("tuple", tmap).
					Msg("failed to delete tuple during factory reset")
				continue
			}
			deleted++
		}

		if nextToken == "" {
			break
		}
		token = nextToken
	}

	log.Info().
		Int("total", total).
		Int("deleted", deleted).
		Int("failed", failed).
		Msg("✅ factory reset tenant completed")

	return
}
