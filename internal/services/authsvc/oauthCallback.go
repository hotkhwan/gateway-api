// internal/services/authsvc/oauthCallback.go
package authsvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
	"github.com/hotkhwan/gateway-api/models/authmod"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/utils/authutil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

func AuthenticateOAuthCode(ctx context.Context, req authmod.OAuthCallbackRequest) (authmod.SigninResponse, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/authsvc", "authenticate.oauthCode", "authsvc", "AuthenticateOAuthCode")
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 1) exchange code -> tokens
	tr, err := authgw.ExchangeAuthorizationCode(ctx, req.Code, req.RedirectUri)
	if err != nil {
		log.Error().Err(err).Msg("❌ Keycloak code exchange failed")
		return authmod.SigninResponse{}, errors.New("unauthorized")
	}

	// 2) decode access token -> claims
	claims, err := authutil.DecodeJWT(tr.AccessToken)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode access token")
		return authmod.SigninResponse{}, err
	}

	// 3) reuse "build response" logic (เหมือน Signin)
	sub := fmt.Sprintf("%v", claims["sub"])
	preferredUsername := fmt.Sprintf("%v", claims["preferred_username"])

	role, _ := claims["role"].(string)
	if strings.TrimSpace(role) == "" {
		role = "user"
	}

	// โหลดเมนูจาก Mongo + merge permify
	menuIDs, coreMenuIDs, perms, meta, err := buildMenusAndPermissions(ctx, claims, role, preferredUsername)
	if err != nil {
		log.Warn().Err(err).Msg("⚠️ buildMenusAndPermissions failed, continue with defaults")
	}

	menusArr := buildMenusArray(menuIDs, coreMenuIDs, perms)

	resp := authmod.SigninResponse{
		Sub:      sub,
		Username: preferredUsername,
		Name:     fmt.Sprintf("%v", claims["name"]),
		Email:    fmt.Sprintf("%v", claims["email"]),
		Role:     role,
		Permissions: authmod.PermissionsBlock{
			Menus: menusArr,
		},
		Meta:         meta,
		Locale:       fmt.Sprintf("%v", claims["locale"]),
		Plant:        claims["plant"],
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
	}

	// 4) hwId / station
	if req.HwID != "" {
		if err := fillStationInfo(ctx, req.HwID, &resp); err != nil {
			return authmod.SigninResponse{}, err
		}
	}

	// 5) keep schema version meta if any
	if sv := config.CurrentSchemaVersion; sv != "" {
		if resp.Meta == nil {
			resp.Meta = map[string]string{}
		}
		resp.Meta["schema_version"] = sv
	}

	return resp, nil
}

// helper outputs for menus/perms
type permItem = authzmod.PermissionSubjectCheckItem
