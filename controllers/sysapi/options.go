// controllers/sysapi/options.go
package sysapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"klynx/models/systemmod"

	"github.com/gofiber/fiber/v2"
)

// ---- Repo interface ที่ต้องมีจาก optionsrepo ----

type OptionsRepo interface {
	// โหลดค่า effective (DB + ENV fallback + defaults)
	LoadEffective(ctx context.Context) (*systemmod.EffectiveConfig, error)
	// อัปเดตแบบ flat fields (จะถูก $set ลงเอกสาร _id=system.setting)
	PatchFlat(ctx context.Context, set map[string]any) error
}

// ---- Controller ----

type OptionsController struct {
	repo                OptionsRepo
	ensureAuditTTLIndex func(ctx context.Context, days int) error
}

func NewOptionsController(repo OptionsRepo, ensureAuditTTLIndex func(ctx context.Context, days int) error) *OptionsController {
	return &OptionsController{
		repo:                repo,
		ensureAuditTTLIndex: ensureAuditTTLIndex,
	}
}

// GET /system/options
func (ctl *OptionsController) GetEffective(c *fiber.Ctx) error {
	eff, err := ctl.repo.LoadEffective(c.Context())
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"code":    "ERROR",
			"message": err.Error(),
			"status":  false,
		})
	}
	return c.JSON(fiber.Map{
		"code":    "SUCCESS",
		"message": "ok",
		"status":  true,
		"data":    eff,
	})
}

// PATCH /system/options
func (ctl *OptionsController) Patch(c *fiber.Ctx) error {
	dec := json.NewDecoder(strings.NewReader(string(c.Body())))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"code":    "BAD_REQUEST",
			"message": "invalid JSON body",
			"status":  false,
		})
	}

	flat := map[string]any{}

	switch v := raw.(type) {
	case map[string]any:
		// 2.1) flat fields
		mergeIfPresentString(v, flat, "auditCaptureResponse")
		mergeIfPresentNumber(v, flat, "auditMaxRespBytes")
		mergeIfPresentBool(v, flat, "auditCaptureJSONOnly")
		if mergeIfPresentNumber(v, flat, "auditRetentionDays") {
			if n, ok := flat["auditRetentionDays"].(int64); ok && n < 1 {
				return badReq(c, "auditRetentionDays must be >= 1")
			}
		}

		mergeIfPresentString(v, flat, "basePath")
		mergeIfPresentNumber(v, flat, "maxRecordRequest")
		mergeIfPresentString(v, flat, "kafkaPublishTimeout")
		mergeIfPresentNumber(v, flat, "kwatchBatchSize")

		// รองรับ flat key ของ kctrl (ทั้งตัวใหญ่/ตัวเล็ก)
		mergeIfPresentNumber(v, flat, "KctrlWatchdogInterval", "kctrlWatchdogInterval")
		mergeIfPresentNumber(v, flat, "KctrlWarnMultiplier", "kctrlWarnMultiplier")
		mergeIfPresentNumber(v, flat, "KctrlOfflineMultiplier", "kctrlOfflineMultiplier")

		mergeIfPresentNumber(v, flat, "kctrlWatchdogInterval", "kctrlWatchdogInterval")
		mergeIfPresentNumber(v, flat, "kctrlWarnMultiplier", "kctrlWarnMultiplier")
		mergeIfPresentNumber(v, flat, "kctrlOfflineMultiplier", "kctrlOfflineMultiplier")

		// 2.2) nested objects
		if a, ok := v["audit"].(map[string]any); ok {
			mergeIfPresentString(a, flat, "captureResponse", "auditCaptureResponse")
			mergeIfPresentNumber(a, flat, "maxRespBytes", "auditMaxRespBytes")
			mergeIfPresentBool(a, flat, "captureJSONOnly", "auditCaptureJSONOnly")
			if mergeIfPresentNumber(a, flat, "retentionDays", "auditRetentionDays") {
				if n, ok := flat["auditRetentionDays"].(int64); ok && n < 1 {
					return badReq(c, "audit.retentionDays must be >= 1")
				}
			}
		}
		if h, ok := v["http"].(map[string]any); ok {
			mergeIfPresentString(h, flat, "basePath", "basePath")
			mergeIfPresentNumber(h, flat, "maxRecordRequest", "maxRecordRequest")
		}
		if k, ok := v["kafka"].(map[string]any); ok {
			mergeIfPresentString(k, flat, "publishTimeout", "kafkaPublishTimeout")
		}
		if kw, ok := v["kwatch"].(map[string]any); ok {
			mergeIfPresentNumber(kw, flat, "batchSize", "kwatchBatchSize")
		}
		if kc, ok := v["kctrl"].(map[string]any); ok {
			// รองรับทั้งตัวเล็ก/ตัวใหญ่ใน nested
			mergeIfPresentNumber(kc, flat, "watchdogInterval", "kctrlWatchdogInterval")
			mergeIfPresentNumber(kc, flat, "WatchdogInterval", "kctrlWatchdogInterval")

			mergeIfPresentNumber(kc, flat, "warnMultiplier", "kctrlWarnMultiplier")
			mergeIfPresentNumber(kc, flat, "WarnMultiplier", "kctrlWarnMultiplier")

			mergeIfPresentNumber(kc, flat, "offlineMultiplier", "kctrlOfflineMultiplier")
			mergeIfPresentNumber(kc, flat, "OfflineMultiplier", "kctrlOfflineMultiplier")
		}
	default:
		return badReq(c, "JSON object required")
	}

	if len(flat) == 0 {
		return badReq(c, "no valid fields to update")
	}

	// validate auditCaptureResponse
	if v, ok := flat["auditCaptureResponse"].(string); ok {
		mode := strings.ToLower(strings.TrimSpace(v))
		switch mode {
		case "none", "errors", "all":
			flat["auditCaptureResponse"] = mode
		default:
			return badReq(c, "auditCaptureResponse must be one of: none|errors|all")
		}
	}

	// บังคับชนิด number >0 / >=0
	if v, ok := flat["auditMaxRespBytes"].(int64); ok && v <= 0 {
		return badReq(c, "auditMaxRespBytes must be > 0")
	}
	if v, ok := flat["maxRecordRequest"].(int64); ok && v <= 0 {
		return badReq(c, "maxRecordRequest must be > 0")
	}
	if v, ok := flat["kwatchBatchSize"].(int64); ok && v < 0 {
		return badReq(c, "kwatchBatchSize must be >= 0")
	}
	if v, ok := flat["kctrlWatchdogInterval"].(int64); ok && v < 0 {
		return badReq(c, "kctrlWatchdogInterval must be >= 0")
	}
	if v, ok := flat["kctrlWarnMultiplier"].(int64); ok && v < 0 {
		return badReq(c, "kctrlWarnMultiplier must be >= 0")
	}
	if v, ok := flat["kctrlOfflineMultiplier"].(int64); ok && v < 0 {
		return badReq(c, "kctrlOfflineMultiplier must be >= 0")
	}
	if v, ok := flat["streamSessionTimeout"].(int64); ok && v < 0 {
		return badReq(c, "kctrlOfflineMultiplier must be >= 0")
	}
	if err := ctl.repo.PatchFlat(c.Context(), flat); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"code":    "ERROR",
			"message": err.Error(),
			"status":  false,
		})
	}

	if v, ok := flat["auditRetentionDays"].(int64); ok && ctl.ensureAuditTTLIndex != nil {
		_ = ctl.ensureAuditTTLIndex(c.Context(), int(v))
	}

	return c.JSON(fiber.Map{
		"code":    "SUCCESS",
		"message": "updated",
		"status":  true,
		"updated": flat,
		"time":    time.Now().UTC(),
	})
}

// ---- helpers ----

func badReq(c *fiber.Ctx, msg string) error {
	return c.Status(http.StatusBadRequest).JSON(fiber.Map{
		"code":    "BAD_REQUEST",
		"message": msg,
		"status":  false,
	})
}

func mergeIfPresentString(src map[string]any, dst map[string]any, inKey string, outKeyOpt ...string) bool {
	outKey := inKey
	if len(outKeyOpt) > 0 {
		outKey = outKeyOpt[0]
	}
	if v, ok := src[inKey]; ok {
		if s, ok2 := v.(string); ok2 {
			dst[outKey] = s
			return true
		}
	}
	return false
}
func mergeIfPresentBool(src map[string]any, dst map[string]any, inKey string, outKeyOpt ...string) bool {
	outKey := inKey
	if len(outKeyOpt) > 0 {
		outKey = outKeyOpt[0]
	}
	if v, ok := src[inKey]; ok {
		if b, ok2 := v.(bool); ok2 {
			dst[outKey] = b
			return true
		}
	}
	return false
}
func mergeIfPresentNumber(src map[string]any, dst map[string]any, inKey string, outKeyOpt ...string) bool {
	outKey := inKey
	if len(outKeyOpt) > 0 {
		outKey = outKeyOpt[0]
	}
	if v, ok := src[inKey]; ok {
		switch n := v.(type) {
		case json.Number:
			if iv, err := n.Int64(); err == nil {
				dst[outKey] = iv
				return true
			}
			// ถ้าเป็น float ก็ปัดลง
			if fv, err := n.Float64(); err == nil {
				dst[outKey] = int64(fv)
				return true
			}
		case float64:
			dst[outKey] = int64(n)
			return true
		case int64:
			dst[outKey] = n
			return true
		case int:
			dst[outKey] = int64(n)
			return true
		}
	}
	return false
}
