// internal/services/authsvc/introspect.go
package authsvc

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/models/authmod"
	"github.com/hotkhwan/gateway-api/utils/authutil"
	"go.opentelemetry.io/otel"
)

func Introspect(ctx context.Context, token string) (*authmod.IntrospectResult, error) {
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/authsvc")
	ctx, span := tracer.Start(ctx, "Auth.Introspect")
	defer span.End()

	log := logger.FromCtx(ctx, "authsvc", "Introspect")

	claims, err := authutil.ValidateJWT(token)
	if err != nil {
		log.Warn().Err(err).Msg("token validation failed")
		return &authmod.IntrospectResult{Active: false}, nil
	}

	expFloat, _ := claims["exp"].(float64)
	exp := int64(expFloat)
	if exp < time.Now().Unix() {
		log.Warn().Int64("exp", exp).Msg("token expired")
		return &authmod.IntrospectResult{Active: false}, nil
	}

	// ✅ ตรวจ revoke ด้วย ctx เดิม (ไม่ใช้ Background)
	if sid := firstNonEmpty(
		getStringClaim(claims, "session_state"),
		getStringClaim(claims, "jti"),
	); sid != "" {
		if exists, _ := config.Redis.Exists(ctx, "revoked:"+sid).Result(); exists > 0 {
			log.Warn().Str("sid", sid).Msg("token revoked by signout")
			return &authmod.IntrospectResult{Active: false}, nil
		}
	}

	subjectID := getStringClaim(claims, "sub")

	res := &authmod.IntrospectResult{
		Active:            true,
		Sub:               subjectID,
		Username:          getStringClaim(claims, "preferred_username"),
		PreferredUsername: getStringClaim(claims, "preferred_username"),
		GivenName:         getStringClaim(claims, "given_name"),
		FamilyName:        getStringClaim(claims, "family_name"),
		Avatar:            getStringClaim(claims, "avatar"),
		Email:             getStringClaim(claims, "email"),
		Locale:            getStringClaim(claims, "locale"),
		Exp:               exp,
		Scope:             getStringClaim(claims, "scope"),
		Role:              getStringClaim(claims, "role"),
		ZoomLevel:         getIntClaim(claims, "zoomLevel"),
		MapLocation:       &authmod.MapLocation{Lat: getFloatClaim(claims, "lat"), Lng: getFloatClaim(claims, "lng")},
	}

	log.Debug().
		Str("sub", res.Sub).
		Str("username", res.Username).
		Int64("exp", res.Exp).
		Msg("token validated successfully")

	return res, nil
}

func getIntClaim(claims map[string]interface{}, key string) int {
	if v, ok := claims[key].(int); ok {
		return v
	}
	return 0
}

func getFloatClaim(claims map[string]interface{}, key string) float64 {
	if v, ok := claims[key].(float64); ok {
		return v
	}
	return 0
}

func getStringClaim(claims map[string]interface{}, key string) string {
	if v, ok := claims[key].(string); ok {
		return v
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
