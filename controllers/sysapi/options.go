// controllers/sysapi/options.go
package sysapi

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/models/systemmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// ---- Repo interface ----

type OptionsRepo interface {
	LoadEffective(ctx context.Context) (*systemmod.EffectiveConfig, error)
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
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.sysapi", "OptionsController.GetEffective", "sysapi", "GetEffective")
	defer end()

	eff, err := ctl.repo.LoadEffective(ctx)
	if err != nil {
		log.Error().Err(err).Msg("load effective config failed")
		return httputil.FailInternal(c, "failed to load config")
	}

	return httputil.Ok(c, eff)
}

// PATCH /system/options
func (ctl *OptionsController) Patch(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.sysapi", "OptionsController.Patch", "sysapi", "Patch")
	defer end()

	dec := json.NewDecoder(strings.NewReader(string(c.Body())))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return httputil.FailBadRequest(c, "invalid JSON body")
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
				return httputil.FailBadRequest(c, "auditRetentionDays must be >= 1")
			}
		}

		mergeIfPresentString(v, flat, "basePath")
		mergeIfPresentNumber(v, flat, "maxRecordRequest")
		mergeIfPresentString(v, flat, "kafkaPublishTimeout")
		mergeIfPresentNumber(v, flat, "kwatchBatchSize")

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
					return httputil.FailBadRequest(c, "audit.retentionDays must be >= 1")
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
			mergeIfPresentNumber(kc, flat, "watchdogInterval", "kctrlWatchdogInterval")
			mergeIfPresentNumber(kc, flat, "WatchdogInterval", "kctrlWatchdogInterval")

			mergeIfPresentNumber(kc, flat, "warnMultiplier", "kctrlWarnMultiplier")
			mergeIfPresentNumber(kc, flat, "WarnMultiplier", "kctrlWarnMultiplier")

			mergeIfPresentNumber(kc, flat, "offlineMultiplier", "kctrlOfflineMultiplier")
			mergeIfPresentNumber(kc, flat, "OfflineMultiplier", "kctrlOfflineMultiplier")
		}
	default:
		return httputil.FailBadRequest(c, "JSON object required")
	}

	if len(flat) == 0 {
		return httputil.FailBadRequest(c, "no valid fields to update")
	}

	// validate auditCaptureResponse
	if v, ok := flat["auditCaptureResponse"].(string); ok {
		mode := strings.ToLower(strings.TrimSpace(v))
		switch mode {
		case "none", "errors", "all":
			flat["auditCaptureResponse"] = mode
		default:
			return httputil.FailBadRequest(c, "auditCaptureResponse must be one of: none|errors|all")
		}
	}

	if v, ok := flat["auditMaxRespBytes"].(int64); ok && v <= 0 {
		return httputil.FailBadRequest(c, "auditMaxRespBytes must be > 0")
	}
	if v, ok := flat["maxRecordRequest"].(int64); ok && v <= 0 {
		return httputil.FailBadRequest(c, "maxRecordRequest must be > 0")
	}
	if v, ok := flat["kwatchBatchSize"].(int64); ok && v < 0 {
		return httputil.FailBadRequest(c, "kwatchBatchSize must be >= 0")
	}
	if v, ok := flat["kctrlWatchdogInterval"].(int64); ok && v < 0 {
		return httputil.FailBadRequest(c, "kctrlWatchdogInterval must be >= 0")
	}
	if v, ok := flat["kctrlWarnMultiplier"].(int64); ok && v < 0 {
		return httputil.FailBadRequest(c, "kctrlWarnMultiplier must be >= 0")
	}
	if v, ok := flat["kctrlOfflineMultiplier"].(int64); ok && v < 0 {
		return httputil.FailBadRequest(c, "kctrlOfflineMultiplier must be >= 0")
	}
	if v, ok := flat["streamSessionTimeout"].(int64); ok && v < 0 {
		return httputil.FailBadRequest(c, "streamSessionTimeout must be >= 0")
	}
	if err := ctl.repo.PatchFlat(ctx, flat); err != nil {
		log.Error().Err(err).Msg("patch config failed")
		return httputil.FailInternal(c, "failed to update config")
	}

	if v, ok := flat["auditRetentionDays"].(int64); ok && ctl.ensureAuditTTLIndex != nil {
		_ = ctl.ensureAuditTTLIndex(ctx, int(v))
	}

	return httputil.Ok(c, fiber.Map{
		"updated": flat,
		"time":    time.Now().UTC(),
	}, "updated")
}

// ---- helpers ----

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
