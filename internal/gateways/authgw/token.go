// internal/gateways/authgw/token.go
package authgw

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type tokenResp struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

func GetAdminAccessToken(ctx context.Context) (string, error) {
	kcURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")
	cid := os.Getenv("KEYCLOAK_CLIENT_ID")
	secret := os.Getenv("KEYCLOAK_CLIENT_SECRET")

	if kcURL == "" || realm == "" || cid == "" || secret == "" {
		return "", errors.New("missing keycloak client env")
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", cid)
	form.Set("client_secret", secret)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(kcURL, "/")+
			"/realms/"+realm+
			"/protocol/openid-connect/token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("failed to get keycloak admin token")
	}

	var tr tokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}

	return tr.AccessToken, nil
}
