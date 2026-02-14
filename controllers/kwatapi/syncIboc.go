// controllers/kwatapi/syncIboc.go
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

// @Summary      Sync IBOC images (enqueue tasks)
// @Description  Enqueue a batch sync task and return 202 immediately (fire-and-forget). Progress is pushed via MQTT/Kafka events.
// @Tags         kwatch
// @Produce      json
// @Success      202 {object} gmod.AcceptedResponse
// @Header       202 {string}  X-Trace-Id   "Trace ID for correlating logs/traces"
// @Header       202 {string}  Location     "Polling URL e.g. /kwatch/jobs/{jobId}"
// @Header       202 {integer} Retry-After  "Client may poll the Location after N seconds"
// @Router       /kwatch/syncIboc [post]
// @Security BearerAuth
func SyncIbocWatchlist(c *fiber.Ctx) error {
	// keep a short timeout only for building the immediate response
	reqCtx, cancel := context.WithTimeout(c.Context(), 60*time.Second)
	defer cancel()

	tr := otel.Tracer("klynx/kwatch")
	reqCtx, span := tr.Start(reqCtx, "kwatapi.SyncIbocWatchlist", trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()

	// correlation id = traceID (fallback to uuid)
	traceID := traceutil.TraceIDFromCtx(reqCtx)
	if traceID == "" {
		traceID = uuid.NewString()
	}
	jobID := traceID

	// send 202 immediately
	resp := gmod.AcceptedResponse{
		Status:  true,
		Code:    "ACCEPTED",
		Message: "Sync IBOC task accepted",
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

	// detach span context from request (avoid cancel/deadline) but keep parent SpanContext
	bg := traceutil.DetachWithParent(reqCtx)

	mode := strings.TrimSpace(c.Query("mode")) // "", "missing", "backfill", "all"
	if mode == "" {
		mode = "missing"
	}

	// fire-and-forget background work
	go func(ctx context.Context, job string, m string) {
		kwatsvc.NotifyIbocProgressStarted(job)
		if err := kwatsvc.EmitSyncIbocTask(ctx, job, m); err != nil {
			kwatsvc.NotifyIbocProgressFailed(job, err.Error())
			return
		}
		kwatsvc.NotifyIbocProgressQueued(job)
	}(bg, jobID, mode)

	return nil
}
