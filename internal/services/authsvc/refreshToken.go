// internal/services/authsvc/refreshToken.go
package authsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/authmod"
	"github.com/hotkhwan/gateway-api/utils/authutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

func RefreshToken(ctx context.Context, refreshToken string) (authmod.SigninResponse, error) {
	// ⬇️ สร้าง child span ของชั้น controller
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/authsvc")
	ctx, span := tracer.Start(ctx, "Auth.RefreshToken")
	defer span.End()

	log := logger.FromCtx(ctx, "authsvc", "RefreshToken")
	log.Debug().Msg("refreshing token")

	form := url.Values{}
	form.Set("client_id", os.Getenv("KEYCLOAK_CLIENT_ID"))
	form.Set("client_secret", os.Getenv("KEYCLOAK_CLIENT_SECRET"))
	form.Set("grant_type", "refresh_token")
	form.Set("scope", "openid")
	form.Set("refresh_token", refreshToken)

	keycloakURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token",
		os.Getenv("KEYCLOAK_URL"), os.Getenv("KEYCLOAK_REALM"))

	// ⬇️ ใช้ http.Client ที่มี otelhttp เพื่อได้ client-span + propagate header
	httpClient := &http.Client{
		Timeout:   5 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, keycloakURL, strings.NewReader(form.Encode()))
	if err != nil {
		log.Error().Err(err).Msg("build request failed")
		return authmod.SigninResponse{}, errors.New("unauthorized")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("request failed")
		return authmod.SigninResponse{}, errors.New("unauthorized")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status", resp.StatusCode).Msg("refresh token http error")
		return authmod.SigninResponse{}, errors.New("unauthorized")
	}

	var tokenData struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenData); err != nil {
		log.Error().Err(err).Msg("decode token response failed")
		return authmod.SigninResponse{}, err
	}

	claims, err := authutil.DecodeJWT(tokenData.AccessToken)
	if err != nil {
		log.Error().Err(err).Msg("decode refreshed token failed")
		return authmod.SigninResponse{}, err
	}

	// ⬇️ อย่า shadow ctx เดิม ให้ทำ sub-timeout โดย derive จาก ctx
	permCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	resourceMap := map[string]struct {
		Type string
		Acts []string
	}{
		"menu_admin_global":      {Type: "menu_admin", Acts: []string{"read"}},
		"menu_management_global": {Type: "menu_management", Acts: []string{"read"}},
		"menu_user_global":       {Type: "menu_user", Acts: []string{"read"}},
		"menu_visitor_global":    {Type: "menu_visitor", Acts: []string{"read"}},
	}

	permifyPermissions, _ := authzsvc.GetUserGlobalPermissions(permCtx, fmt.Sprint(claims["sub"]), resourceMap)

	flatPermissions := make([]string, 0, 8)
	for res, acts := range permifyPermissions {
		for _, a := range acts {
			flatPermissions = append(flatPermissions, res+":"+a) // เช่น "menu_admin_global:read"
		}
	}

	// (ถ้าต้อง flatten permissions ใส่ที่นี่ตามต้องการ)

	log.Debug().Str("sub", fmt.Sprint(claims["sub"])).Msg("token refreshed successfully")

	return authmod.SigninResponse{
		Sub:          fmt.Sprint(claims["sub"]),
		Username:     fmt.Sprint(claims["preferred_username"]),
		Name:         fmt.Sprint(claims["name"]),
		Email:        fmt.Sprint(claims["email"]),
		Role:         fmt.Sprint(claims["role"]),
		Locale:       fmt.Sprint(claims["locale"]),
		Plant:        claims["plant"], // ถ้าเป็น string ก็ใช้ fmt.Sprint เช่นกัน
		AccessToken:  tokenData.AccessToken,
		RefreshToken: tokenData.RefreshToken,
	}, nil
}
