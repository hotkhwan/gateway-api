package authgw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type OAuthTokenResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func ExchangeAuthorizationCode(ctx context.Context, code string, redirectUri string) (OAuthTokenResult, error) {
	kcURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")

	feClientID := os.Getenv("KEYCLOAK_FE_CLIENT_ID")
	feSecret := os.Getenv("KEYCLOAK_FE_CLIENT_SECRET") // ว่างได้ถ้า public client

	if kcURL == "" || realm == "" || feClientID == "" {
		return OAuthTokenResult{}, errors.New("missing keycloak env (KEYCLOAK_URL/REALM/FE_CLIENT_ID)")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", feClientID)
	if strings.TrimSpace(feSecret) != "" {
		form.Set("client_secret", feSecret)
	}
	form.Set("code", code)
	form.Set("redirect_uri", redirectUri)

	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token",
		strings.TrimRight(kcURL, "/"), realm)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return OAuthTokenResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return OAuthTokenResult{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return OAuthTokenResult{}, fmt.Errorf("keycloak token exchange failed (%d): %s", resp.StatusCode, string(body))
	}

	var out OAuthTokenResult
	if err := json.Unmarshal(body, &out); err != nil {
		return OAuthTokenResult{}, err
	}
	if out.AccessToken == "" {
		return OAuthTokenResult{}, errors.New("missing access_token from keycloak")
	}
	return out, nil
}
