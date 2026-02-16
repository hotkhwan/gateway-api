// controllers/authapi/introspect.go
package authapi

import (
	"strings"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/authsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// Introspect godoc
// @Summary      Introspect JWT Token (Local JWKS Verify)
// @Description  ตรวจสอบความถูกต้องของ JWT จาก Authorization Header
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        Authorization header string true "Bearer <JWT>"
// @Success      200 {object} gmod.SuccessDetailResponseIntrospect
// @Failure      400 {object} gmod.ApiErrorResponse
// @Failure      401 {object} gmod.ApiErrorResponse
// @Failure      500 {object} gmod.ApiErrorResponse
// @Router       /auth/introspect [get]
// @Security     BearerAuth
func Introspect(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/authapi")
	ctx, span := tracer.Start(ctx, "Auth.Introspect")
	defer span.End()

	log := logger.FromCtx(ctx, "authapi", "Introspect")

	authHeader := strings.TrimSpace(c.Get("Authorization"))
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return httputil.FailBadRequestReason(
			c,
			"Authorization header missing or invalid",
			"INVALID_AUTH_HEADER",
		)
	}

	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if token == "" {
		return httputil.FailBadRequestReason(
			c,
			"Authorization header missing or invalid",
			"EMPTY_BEARER_TOKEN",
		)
	}

	res, err := authsvc.Introspect(ctx, token)
	if err != nil {
		log.Error().Err(err).Msg("introspect failed")
		return httputil.FailInternalReason(
			c,
			"internal server error",
			"INTROSPECT_ERROR",
		)
	}

	if !res.Active {
		return httputil.FailUnauthorized(
			c,
			"invalid or expired token",
			httputil.ErrorDetails{
				"reason": "TOKEN_INACTIVE",
			},
		)
	}

	log.Debug().Msg("token introspect successful")

	return c.Status(fiber.StatusOK).JSON(gmod.SuccessDetailResponse[gmod.IntrospectDetail]{
		Detail: gmod.IntrospectDetail{
			Active:            res.Active,
			Sub:               res.Sub,
			Given_name:        res.GivenName,
			Family_name:       res.FamilyName,
			PreferredUsername: res.PreferredUsername,
			Username:          res.Username,
			Avatar:            res.Avatar,
			Email:             res.Email,
			Locale:            res.Locale,
			Exp:               res.Exp,
			Scope:             res.Scope,
			Role:              res.Role,
			Permissions:       res.Permissions,
			ZoomLevel:         res.ZoomLevel,
			MapLocation:       gmod.MapLocation{Lat: res.MapLocation.Lat, Lng: res.MapLocation.Lng},
		},
		Status: true,
	})
}
