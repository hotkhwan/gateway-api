// controllers/optapi/putListOptions.go
package optapi

import (
	"github.com/gofiber/fiber/v2"

	"github.com/hotkhwan/gateway-api/internal/services/optsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// PUT /options  (kind = "list" ตายตัว)
func PutOptions(c *fiber.Ctx) error {
	_, end, log := traceutil.StartLite(c.UserContext(), "gateway.optapi", "PutOptions", "optapi", "PutOptions")
	defer end()

	kind := "list"

	// payload เป็น map[namespace]map[string]any
	var payload map[string]map[string]any
	if err := c.BodyParser(&payload); err != nil {
		return httputil.FailBadRequest(c, "invalid request body")
	}
	if len(payload) == 0 {
		return httputil.FailBadRequest(c, "empty payload")
	}

	if err := optsvc.UpsertOptions(c.Context(), kind, payload); err != nil {
		log.Error().Err(err).Msg("failed to upsert options")
		return httputil.FailInternal(c, "failed to upsert options")
	}

	return httputil.MessageOK(c, "Options upserted successfully")
}
