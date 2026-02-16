// internal/services/authsvc/auth.go
package authsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/utils/authutil"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

func ResetPassword(ctx context.Context, _ string, accessToken string, newPassword string) (map[string]interface{}, error) {
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/authsvc")
	ctx, span := tracer.Start(ctx, "Auth.ResetPassword")
	defer span.End()

	log := logger.FromCtx(ctx, "authsvc", "ResetPassword")
	log.Info().Msg("resetting password")

	adminToken, err := authutil.GetAdminAccessToken()
	if err != nil {
		log.Error().Err(err).Msg("get admin token failed")
		return nil, err
	}

	claims, err := authutil.DecodeJWT(accessToken)
	if err != nil {
		log.Error().Err(err).Msg("decode user token failed")
		return nil, err
	}
	userSub := fmt.Sprint(claims["sub"]) // safe stringify

	url := fmt.Sprintf("%s/admin/realms/%s/users/%s/reset-password",
		os.Getenv("KEYCLOAK_URL"), os.Getenv("KEYCLOAK_REALM"), userSub)

	payload := map[string]any{
		"type":      "password",
		"value":     newPassword,
		"temporary": false,
	}
	jsonBody, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(jsonBody))
	if err != nil {
		log.Error().Err(err).Msg("build request failed")
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("request failed")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		log.Error().Int("status", resp.StatusCode).Msg("password reset failed")
		return nil, fmt.Errorf("reset failed: %s", resp.Status)
	}

	log.Debug().Str("sub", userSub).Msg("password reset successful")
	return map[string]interface{}{"message": "Password reset successful"}, nil
}
