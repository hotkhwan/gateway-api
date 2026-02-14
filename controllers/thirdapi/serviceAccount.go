// controllers/thirdapi/serviceAccount.go
package thirdapi

import (
	"os"
	"strings"

	"klynx/internal/logger"
	"klynx/internal/services/thirdsvc"
	"klynx/models/thirdmod"
	"klynx/utils/httputil"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// @Summary Create Service Account Client (Public, with Admin API Key)
// @Tags Third-Party
// @Accept json
// @Produce json
// @Param X-Admin-Secret header string true "Admin API Key"
// @Param data body thirdmod.CreateServiceAccountReq true "Create client for service account"
// @Success 200 {object} gmod.SuccessDataResponse
// @Failure 400 {object} gmod.ApiErrorResponse
// @Failure 401 {object} gmod.ApiErrorResponse
// @Failure 500 {object} gmod.ApiErrorResponse
// @Router /token/api/serviceAccount [post]
func CreateServiceAccountClient(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/thirdapi")
	ctx, span := tracer.Start(ctx, "ThirdAPI.CreateServiceAccountClient")
	defer span.End()

	log := logger.FromCtx(ctx, "thirdapi", "CreateServiceAccountClient")

	// ✅ ตรวจสอบ Admin Secret (กันคนเรียกมั่ว)
	expectedSecret := strings.TrimSpace(os.Getenv("ADMIN_API_KEY"))
	got := strings.TrimSpace(c.Get("X-Admin-Secret"))

	if expectedSecret != "" {
		if got == "" || got != expectedSecret {
			return httputil.FailUnauthorized(c, "invalid admin secret")
		}
	} else {
		// best practice: ถ้าไม่ได้ตั้งค่า ADMIN_API_KEY = endpoint นี้ควรถูกปิด
		log.Error().Msg("ADMIN_API_KEY is empty - serviceAccount endpoint is unsafe")
		return httputil.FailInternal(c, "internal server error", httputil.ErrorDetails{
			"reason": "ADMIN_API_KEY_NOT_CONFIGURED",
		})
	}

	var req thirdmod.CreateServiceAccountReq
	if err := c.BodyParser(&req); err != nil {
		log.Warn().Err(err).Msg("invalid request body")
		return httputil.FailBadRequest(c, "invalid json body")
	}

	if strings.TrimSpace(req.Description) == "" {
		req.Description = "auto created via API"
	}

	clientID, clientSecret, err := thirdsvc.GenerateRandomClient(ctx, req.Description)
	if err != nil {
		log.Error().Err(err).Msg("create service-account client failed")
		// อันนี้ไม่ใช่ bad request ของ user แล้ว ควรเป็น 500
		return httputil.FailInternal(c, "internal server error", httputil.ErrorDetails{
			"reason": "SERVICE_ACCOUNT_CREATE_FAILED",
		})
	}

	return httputil.Ok(c, thirdmod.CreateServiceAccountRes{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	})
}
