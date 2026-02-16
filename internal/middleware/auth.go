// internal/middleware/auth.go
package middleware

import (
	"errors"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/utils/authutil"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

// ExtractBearerToken ดึง Bearer Token ออกมาจาก Header Authorization
func ExtractBearerToken(c *fiber.Ctx) (string, error) {
	log := logger.FromCtx(c.UserContext(), "middleware", "authz-ExtractBearerToken")
	authHeader := c.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		log.Warn().
			Str("authHeader", authHeader).
			Str("ip", c.IP()).
			Msg("missing_or_invalid_authorization_header")
		return "", errors.New("missing or invalid authorization header")
	}
	return strings.TrimPrefix(authHeader, "Bearer "), nil
}

// AuthBearer Middleware สำหรับ Validate Token กับ Keycloak

func AuthBearer() fiber.Handler {
	return func(c *fiber.Ctx) error {
		log := logger.FromCtx(c.UserContext(), "middleware", "authz-AuthBearer")

		token, err := ExtractBearerToken(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    "UNAUTHORIZED",
				"message": err.Error(),
				"status":  false,
			})
		}

		claims, err := authutil.ValidateJWT(token)
		if err != nil {
			// ... ของเดิม
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    "INVALID_TOKEN",
				"message": err.Error(),
				"status":  false,
			})
		}

		userId := getStr(claims["sub"])
		username := getStr(claims["preferred_username"])
		if username == "" {
			username = getStr(claims["name"])
		}

		// ✅ tenantId = realm
		// แนะนำอ่านจาก issuer (iss) เพื่อไม่ผูกกับ ENV
		tenantId := authutil.ExtractRealmFromIssuer(getStr(claims["iss"]))
		// fallback ถ้าจำเป็น
		if tenantId == "" {
			tenantId = getStr(claims["realm"]) // เผื่อมี custom claim
		}

		log.Debug().
			Str("sub", userId).
			Str("tenantId", tenantId).
			Str("username", username).
			Str("ip", c.IP()).
			Msg("token_validated")

		// ✅ ของเดิม
		c.Locals("user", claims)

		// ✅ ของใหม่ (additive ไม่กระทบ legacy)
		c.Locals("userId", userId)
		c.Locals("tenantId", tenantId)

		return c.Next()
	}
}

func AuthBearerOrCookie() fiber.Handler {
	return func(c *fiber.Ctx) error {
		auth := c.Get("Authorization")
		if auth == "" {
			tok := c.Cookies("KAPI_TOKEN")
			if tok != "" {
				c.Request().Header.Set("Authorization", "Bearer "+tok)
			}
		}
		return AuthBearer()(c) // ของเดิม
	}
}

// RequireRoles ตรวจสอบ role ของผู้ใช้ใน JWT
func RequireRoles(roles []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		log := logger.FromCtx(c.UserContext(), "middleware", "authz-RequireRoles")

		claims, ok := c.Locals("user").(map[string]interface{})
		if !ok {
			log.Warn().
				Str("ip", c.IP()). // เพิ่ม IP address
				Str("traceID", c.Get("X-Trace-ID")).
				Msg("❌ [Auth] Missing user context")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Missing user context"})
		}

		userRole, ok := claims["role"].(string)
		if !ok {
			log.Warn().
				Str("ip", c.IP()). // เพิ่ม IP address
				Str("traceID", c.Get("X-Trace-ID")).
				Str("user_id", getStr(claims["sub"])).                 // เพิ่ม User ID
				Str("username", getStr(claims["preferred_username"])). // เพิ่ม Username
				Msg("❌ [Auth] Role not found in token")
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "Role not found in token"})
		}

		for _, r := range roles {
			if r == userRole {
				log.Info().
					Str("userRole", userRole).
					Str("sub", getStr(claims["sub"])).
					Str("username", getStr(claims["preferred_username"])).
					Str("ip", c.IP()).
					Msg("✅ [Auth] Access granted")
				return c.Next()
			}
		}

		log.Warn().
			Str("userRole", userRole).
			Str("sub", getStr(claims["sub"])).
			Str("username", getStr(claims["preferred_username"])).
			Str("ip", c.IP()).
			Str("traceID", c.Get("X-Trace-ID")).
			Msg("❌ [Auth] Forbidden - role mismatch")
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "Forbidden"})
	}
}

// CurrentUser ดึงผู้ใช้จาก context
func CurrentUser(c *fiber.Ctx) (map[string]interface{}, error) {
	log := logger.FromCtx(c.UserContext(), "middleware", "authz-CurrentUser")

	switch v := c.Locals("user").(type) {
	case map[string]interface{}:
		return v, nil
	case jwt.MapClaims:
		return map[string]interface{}(v), nil
	default:
		log.Warn().Str("ip", c.IP()).Msg("no_user_in_context")
		return nil, errors.New("no user in context")
	}
}

// CurrentUserID ดึง userId ("sub") จาก context
func CurrentUserID(c *fiber.Ctx) (string, error) {
	claims, err := CurrentUser(c)
	if err != nil {
		return "", err
	}
	if sub, ok := claims["sub"].(string); ok && sub != "" {
		return sub, nil
	}
	return "", errors.New("missing sub in token")
}

// getStr Helper สำหรับ type assert
func getStr(val interface{}) string {
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}
