// controllers/kwatapi/syncIbocById.go
package kwatapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"klynx/internal/services/kwatsvc"
	"klynx/models/gmod"
	"klynx/utils/traceutil"
)

type syncIdsRequest struct {
	// รายการ _id (hex) ของเอกสาร watchlist ที่ต้องการ sync
	IDs []string `json:"ids"`
	// โหมดปลายทาง: "prod" | "dev" | "both" (default="prod")
	Env string `json:"env"`
	// โหมดย่อย: "missing" | "backfill" | "all" (ไม่บังคับ; ถ้าไม่ระบุจะเป็น "byIds")
	Mode string `json:"mode"`
}

// @Summary      Sync IBOC by IDs (enqueue tasks)
// @Description  Enqueue sync tasks for specific _id list (fire-and-forget). Progress via MQTT/Kafka.
// @Tags         kwatch
// @Accept       json
// @Produce      json
// @Param        body  body  syncIdsRequest  true  "IDs and env"
// @Success      202 {object} gmod.AcceptedResponse
// @Header       202 {string}  X-Trace-Id   "Trace ID for correlating logs/traces"
// @Header       202 {string}  Location     "Polling URL e.g. /kwatch/jobs/{jobId}"
// @Header       202 {integer} Retry-After  "Client may poll the Location after N seconds"
// @Router       /kwatch/syncIboc/ids [post]
// @Security     BearerAuth
func SyncIbocByIDs(c *fiber.Ctx) error {
	req := syncIdsRequest{}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid JSON body")
	}
	if len(req.IDs) == 0 {
		return fiber.NewError(http.StatusBadRequest, "ids is required")
	}
	// กันยิงหนักเกินไปในครั้งเดียว (ปรับได้)
	if len(req.IDs) > 10000 {
		return fiber.NewError(http.StatusBadRequest, "too many ids; max 10000")
	}

	env := strings.ToLower(strings.TrimSpace(req.Env))
	if env == "" {
		env = "prod"
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "byids"
	}

	// short timeout เฉพาะทำ response
	reqCtx, cancel := context.WithTimeout(c.Context(), 60*time.Second)
	defer cancel()

	tr := otel.Tracer("klynx/kwatch")
	reqCtx, span := tr.Start(reqCtx, "kwatapi.SyncIbocByIDs", trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()

	traceID := traceutil.TraceIDFromCtx(reqCtx)
	if traceID == "" {
		traceID = uuid.NewString()
	}
	jobID := traceID

	resp := gmod.AcceptedResponse{
		Status:  true,
		Code:    "ACCEPTED",
		Message: "Sync IBOC(by IDs) task accepted",
		Detail: gmod.JobInfo{
			JobID:     jobID,
			TraceID:   traceID,
			MQTTTopic: "ui/msg/watchlist.iboc." + jobID,
		},
	}
	c.Set("X-Trace-Id", traceID)
	c.Set("Location", "/kwatch/jobs/"+jobID)
	c.Set("Retry-After", "3")
	_ = c.Status(http.StatusAccepted).JSON(resp)

	// detach context เพื่อไม่ผูกกับ request timeout
	bg := traceutil.DetachWithParent(reqCtx)

	go func(ctx context.Context, job string, ids []string, env, mode string) {
		kwatsvc.NotifyIbocProgressStarted(job)

		var errs []error
		switch env {
		case "prod":
			if err := kwatsvc.EmitSyncIbocTaskByIDs(ctx, job, mode, ids); err != nil {
				errs = append(errs, err)
			}
		case "dev":
			if err := kwatsvc.EmitSyncIbocDevTaskByIDs(ctx, job, mode, ids); err != nil {
				errs = append(errs, err)
			}
		case "both":
			if err := kwatsvc.EmitSyncIbocTaskByIDs(ctx, job, mode, ids); err != nil {
				errs = append(errs, err)
			}
			if err := kwatsvc.EmitSyncIbocDevTaskByIDs(ctx, job, mode, ids); err != nil {
				errs = append(errs, err)
			}
		default:
			errs = append(errs, fiber.NewError(http.StatusBadRequest, "env must be prod|dev|both"))
		}

		if len(errs) > 0 {
			kwatsvc.NotifyIbocProgressFailed(job, errs[0].Error())
			return
		}
		kwatsvc.NotifyIbocProgressQueued(job)
	}(bg, jobID, dedupTrim(req.IDs), env, mode)

	return nil
}

func dedupTrim(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
