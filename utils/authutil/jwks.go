// utils/authutil/jwks.go
package authutil

import (
	"github.com/hotkhwan/gateway-api/internal/logger"
	"os"
	"time"

	"github.com/MicahParks/keyfunc"
	"github.com/golang-jwt/jwt/v4"
)

var JWKS *keyfunc.JWKS

// เรียกตอนเริ่มแอป
func InitJWKS(jwksURL string) error {
	refresh := os.Getenv("KEYCLOAK_JWKS_REFRESH") == "true"

	options := keyfunc.Options{}
	if refresh {
		options.RefreshInterval = 15 * time.Minute
		options.RefreshErrorHandler = func(err error) {
			log := logger.WithMeta("usrsvc", "ListUsers")
			log.Err(err).Msg("❌ Failed to refresh JWKS")
		}
	}

	var err error
	JWKS, err = keyfunc.Get(jwksURL, options)
	return err
}

// ValidateJWT ตรวจสอบ JWT และคืน claims เสมอ ถ้า parse token ได้
func ValidateJWT(tokenString string) (jwt.MapClaims, error) {
	// Parse token โดยไม่สนใจว่า Valid หรือไม่ (เพื่อดึง claims มาก่อน)
	token, err := jwt.Parse(tokenString, JWKS.Keyfunc)

	// ลองดึง claims ออกมา ถ้า parse ได้
	var claims jwt.MapClaims
	if token != nil {
		if c, ok := token.Claims.(jwt.MapClaims); ok {
			claims = c
		}
	}

	// ตรวจสอบความถูกต้องของ token จริงๆ จังๆ
	if err != nil { // มี error ในการ parse หรือ validate เช่น หมดอายุ
		return claims, err // คืน claims ที่ดึงได้ (อาจจะ nil หรือมีค่า) พร้อมกับ error
	}
	if !token.Valid { // token ไม่ Valid ด้วยเหตุผลอื่นๆ เช่น signature ไม่ตรง
		return claims, jwt.ErrInvalidKey // คืน claims พร้อม error indicating invalid key
	}

	// ถ้าทุกอย่างถูกต้อง
	return claims, nil
}
