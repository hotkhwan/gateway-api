// internal/services/authsvc/signout.go
// internal/services/authsvc/signout.go
package authsvc

import (
	"context"
	"fmt"
	"io"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/utils/authutil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"net/http"
	"os"
	"time"
)

// Signout ทำการ logout ผู้ใช้จาก Keycloak และ blacklist session ใน Redis
func Signout(ctx context.Context, accessToken string) (map[string]interface{}, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authsvc",
		"authenticate.signout",
		"authsvc", "Signout",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	log.Info().Msg("🚪 Signing out user")

	// ✅ 1) ขอ admin token เพื่อเรียก Keycloak Admin API
	adminToken, err := authutil.GetAdminAccessToken()
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to get admin token")
		return nil, err
	}

	// ✅ 2) Decode JWT ของ user
	claims, err := authutil.DecodeJWT(accessToken)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode user token")
		return nil, err
	}

	// ปลอดภัยด้วย type assertion แบบ ok
	sub, _ := claims["sub"].(string)
	sid, _ := claims["session_state"].(string)

	var exp int64
	if v, ok := claims["exp"].(float64); ok {
		exp = int64(v)
	}

	// ✅ 3) เรียก Keycloak Logout API
	url := fmt.Sprintf("%s/admin/realms/%s/users/%s/logout",
		os.Getenv("KEYCLOAK_URL"), os.Getenv("KEYCLOAK_REALM"), sub)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to build signout request")
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("❌ Signout request failed")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		log.Error().
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("❌ Signout failed")
		return nil, fmt.Errorf("logout failed: %s - %s", resp.Status, string(body))
	}

	// ✅ 4) Blacklist token ใน Redis (revoked:<sid>) เพื่อให้ Introspect() เช็คได้
	if sid != "" {
		ctx := context.Background()
		ttl := time.Until(time.Unix(exp, 0))
		if ttl <= 0 {
			ttl = time.Minute // fallback 1 นาที
		}

		key := "revoked:" + sid
		if err := config.Redis.Set(ctx, key, 1, ttl).Err(); err != nil {
			log.Warn().Err(err).Msg("⚠️ Failed to set revoked token in Redis")
		} else {
			log.Debug().Str("key", key).Dur("ttl", ttl).Msg("🛑 Token blacklisted")
		}
	}

	log.Debug().Str("sub", sub).Str("sid", sid).Msg("✅ Signout successful + Blacklisted")

	// ✅ 5) ตอบกลับเป็น format มาตรฐาน
	return map[string]interface{}{
		"code":    "SUCCESS",
		"status":  true,
		"message": "Signout successful",
	}, nil
}
