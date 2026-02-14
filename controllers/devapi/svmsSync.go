// controllers/devapi/svmsSync.go
package devapi

import (
	"context"
	"os"
	"time"

	"klynx/config"
	"klynx/internal/services/devsync"
	"klynx/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

type svmsSyncReq struct {
	BaseURL  string `json:"baseUrl"`
	User     string `json:"user"`
	Pass     string `json:"pass"`
	PageSize int    `json:"pageSize"`
}

func DeviceSyncFromSVMS(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/devapi", "devices.DeviceSyncFromSVMS", "devapi", "DeviceSyncFromSVMS")
	defer span.End()

	var req svmsSyncReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"code": "INVALID_BODY", "message": err.Error(), "status": false})
	}
	if req.BaseURL == "" || req.User == "" || req.Pass == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"code": "MISSING_FIELDS", "message": "baseUrl, user, pass required", "status": false})
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	total, upserts, err := devsync.SyncFromSVMS(ctx, devsync.SyncConfig{
		BaseURL:  req.BaseURL,
		User:     req.User,
		Pass:     req.Pass,
		PageSize: req.PageSize,
	}, db)
	if err != nil {
		log.Error().Err(err).Msg("SVMS sync failed")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code":    "SYNC_FAILED",
			"message": err.Error(),
			"status":  false,
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"code":    "SUCCESS",
		"message": "SVMS camera sync finished",
		"status":  true,
		"details": fiber.Map{
			"total_seen": total,
			"upserts":    upserts,
		},
	})
}
