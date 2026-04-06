// controllers/devapi/svmsSync.go
package devapi

import (
	"context"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/services/devsync"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v3"
)

type svmsSyncReq struct {
	BaseURL  string `json:"baseUrl"`
	User     string `json:"user"`
	Pass     string `json:"pass"`
	PageSize int    `json:"pageSize"`
}

func DeviceSyncFromSVMS(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.devapi", "DeviceSyncFromSVMS", "devapi", "DeviceSyncFromSVMS")
	defer end()

	var req svmsSyncReq
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "Invalid request body")
	}
	if req.BaseURL == "" || req.User == "" || req.Pass == "" {
		return httputil.FailBadRequest(c, "baseUrl, user, pass required")
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
		log.Error().Err(err).Msg("❌ [DeviceSyncFromSVMS] SVMS sync failed")
		return httputil.FailInternal(c, "SVMS sync failed")
	}

	return httputil.Ok(c, fiber.Map{
		"total_seen": total,
		"upserts":    upserts,
	}, "SVMS camera sync finished")
}
