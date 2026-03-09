// controllers/kwatapi/syncIboc.go
package kwatapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/hotkhwan/gateway-api/internal/services/kwatsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// @Summary      Sync IBOCDEV ids (enqueue tasks)
// @Description  Enqueue a batch sync task for IBOCDEV and return 202 immediately (fire-and-forget). Progress is pushed via MQTT/Kafka events.
// @Tags         kwatch
// @Produce      json
// @Success      202 {object} gmod.AcceptedResponse
// @Header       202 {string}  X-Trace-Id   "Trace ID for correlating logs/traces"
// @Header       202 {string}  Location     "Polling URL e.g. /kwatch/jobs/{jobId}"
// @Header       202 {integer} Retry-After  "Client may poll the Location after N seconds"
// @Router       /kwatch/syncIbocDev [post]
// @Security BearerAuth
func SyncIbocDevWatchlist(c *fiber.Ctx) error {
	// short timeout for building response only
	baseCtx, cancel := context.WithTimeout(c.UserContext(), 60*time.Second)
	defer cancel()

	reqCtx, end, _ := traceutil.StartLite(baseCtx, "gateway.kwatapi", "SyncIbocDev.SyncIbocDevWatchlist", "kwatapi", "SyncIbocDevWatchlist")
	defer end()

	// correlation id = traceID (fallback to uuid)
	traceID := traceutil.TraceIDFromCtx(reqCtx)
	if traceID == "" {
		traceID = uuid.NewString()
	}
	jobID := traceID

	// 202 Accepted right away
	resp := gmod.AcceptedResponse{
		Status:  true,
		Code:    "ACCEPTED",
		Message: "Sync IBOCDEV task accepted",
		Details: gmod.JobInfo{
			JobID:     jobID,
			TraceID:   traceID,
			MQTTTopic: "ui/msg/watchlist.ibocdev." + jobID,
		},
	}
	c.Set("X-Trace-Id", traceID)
	c.Set("Location", "/kwatch/jobs/"+jobID)
	c.Set("Retry-After", "3")
	_ = c.Status(http.StatusAccepted).JSON(resp)

	// detach from request deadline but keep parent span context
	bg := traceutil.DetachWithParent(reqCtx)
	mode := strings.TrimSpace(c.Query("mode"))
	if mode == "" {
		mode = "missing"
	}
	// fire-and-forget background work
	go func(ctx context.Context, job string, m string) {
		kwatsvc.NotifyIbocDevProgressStarted(job)
		if err := kwatsvc.EmitSyncIbocDevTask(ctx, job, m); err != nil {
			kwatsvc.NotifyIbocDevProgressFailed(job, err.Error())
			return
		}
		kwatsvc.NotifyIbocDevProgressQueued(job)
	}(bg, jobID, mode)

	return nil
}
