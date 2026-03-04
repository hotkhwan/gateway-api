// controllers/ingestapi/ingest.go
package ingestapi

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/hotkhwan/gateway-api/internal/services/ingestsvc"
)

type IngestController struct {
	service *ingestsvc.IngestService
}

func NewIngestController(svc *ingestsvc.IngestService) *IngestController {
	if svc == nil {
		panic("IngestController: service required")
	}
	return &IngestController{service: svc}
}

// Ingest godoc
// @Summary Ingest raw event (no JWT, hot path)
// @Description POST event payload from device/3rd-party. Rate limited per org + IP.
// @Tags Ingest
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Success 202 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 413 {object} map[string]interface{}
// @Failure 429 {object} map[string]interface{}
// @Router /events/{orgId} [post]
func (ctrl *IngestController) Ingest(c *fiber.Ctx) error {
	orgId := strings.TrimSpace(c.Params("orgId"))
	if orgId == "" {
		return c.Status(400).JSON(fiber.Map{
			"code":    "BAD_REQUEST",
			"message": "orgId is required",
			"status":  false,
		})
	}

	body := c.Body()
	sourceIp := c.IP()
	contentType := c.Get("Content-Type", "application/json")

	result, err := ctrl.service.Ingest(c.UserContext(), orgId, sourceIp, contentType, body)
	if err != nil {
		switch {
		case errors.Is(err, ingestsvc.ErrOrgNotFound):
			return c.Status(404).JSON(fiber.Map{
				"code":    "NOT_FOUND",
				"message": "organization not found",
				"status":  false,
			})
		case errors.Is(err, ingestsvc.ErrRateLimited):
			return c.Status(429).JSON(fiber.Map{
				"code":    "TOO_MANY_REQUESTS",
				"message": "rate limit exceeded",
				"status":  false,
			})
		case errors.Is(err, ingestsvc.ErrPayloadTooLarge):
			return c.Status(413).JSON(fiber.Map{
				"code":    "PAYLOAD_TOO_LARGE",
				"message": "payload exceeds 256KB limit",
				"status":  false,
			})
		case errors.Is(err, ingestsvc.ErrEmptyBody):
			return c.Status(400).JSON(fiber.Map{
				"code":    "BAD_REQUEST",
				"message": "request body is empty",
				"status":  false,
			})
		default:
			return c.Status(500).JSON(fiber.Map{
				"code":    "INTERNAL_ERROR",
				"message": "internal server error",
				"status":  false,
			})
		}
	}

	// Check if device is locked (has pending event)
	if result.Locked {
		return c.Status(423).JSON(fiber.Map{
			"code":        "DEVICE_PENDING_LOCKED",
			"status":      false,
			"message":     result.LockMessage,
			"deviceKey":   result.DeviceKey,
			"pendingEventId": result.EventId,
		})
	}

	return c.Status(202).JSON(fiber.Map{
		"code":       "ACCEPTED",
		"status":     true,
		"message":    "event accepted",
		"eventId":    result.EventId,
		"receivedAt": result.ReceivedAt,
	})
}
