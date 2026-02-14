// controllers/optapi/putListOptions.go
package optapi

import (
	"github.com/gofiber/fiber/v2"

	"klynx/internal/services/optsvc"
	"klynx/models/gmod"
	"klynx/utils/httputil"
)

// PUT /options  (kind = "list" ตายตัว)
func PutOptions(c *fiber.Ctx) error {
	kind := "list"

	// payload เป็น map[namespace]map[string]any
	var payload map[string]map[string]any
	if err := c.BodyParser(&payload); err != nil {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "invalid request body")
	}
	if len(payload) == 0 {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "empty payload")
	}

	if err := optsvc.UpsertOptions(c.Context(), kind, payload); err != nil {
		return httputil.FailInternalReason(c, "internal server error", "FAILED_TO_UPSERT_OPTIONS")
	}

	return gmod.SendMessageOK(c, "SUCCESS", "Options upserted successfully")
}
