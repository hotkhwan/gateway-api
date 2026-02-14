// utils/authn/token.go
package authutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	adminTokenCache     string
	adminTokenExpiresAt time.Time
	adminTokenMu        sync.Mutex
)

func ExtractToken(header string) string {
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimPrefix(header, "Bearer ")
	}
	return ""
}

func IntrospectToken(token string) (bool, error) {
	clientID := os.Getenv("KEYCLOAK_CLIENT_ID")
	clientSecret := os.Getenv("KEYCLOAK_CLIENT_SECRET")
	realm := os.Getenv("KEYCLOAK_REALM")
	introspectURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token/introspect",
		os.Getenv("KEYCLOAK_URL"), realm)

	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("token", strings.TrimPrefix(token, "Bearer "))

	req, _ := http.NewRequest("POST", introspectURL, strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	return result.Active, nil
}

// ใช้ service account ของ client (gw) เพื่อขอ admin access token
func GetAdminAccessToken() (string, error) {
	adminTokenMu.Lock()
	defer adminTokenMu.Unlock()

	// ใช้ cache ถ้ายังไม่หมดอายุ
	if adminTokenCache != "" && time.Now().Before(adminTokenExpiresAt) {
		return adminTokenCache, nil
	}

	clientID := os.Getenv("KEYCLOAK_CLIENT_ID")
	clientSecret := os.Getenv("KEYCLOAK_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return "", fmt.Errorf("KEYCLOAK_CLIENT_ID/SECRET not set")
	}

	form := url.Values{}
	form.Add("grant_type", "client_credentials")
	form.Add("client_id", clientID)
	form.Add("client_secret", clientSecret)

	realm := os.Getenv("KEYCLOAK_REALM")
	if realm == "" {
		realm = "master"
	}

	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token",
		os.Getenv("KEYCLOAK_URL"), realm)

	resp, err := http.PostForm(tokenURL, form)
	if err != nil {
		return "", fmt.Errorf("request admin token (client_credentials) error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("admin token failed (client_credentials): %s - %s", resp.Status, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode admin token response error: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("admin token empty (client_credentials)")
	}

	adminTokenCache = tokenResp.AccessToken
	adminTokenExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn-10) * time.Second)

	return adminTokenCache, nil
}
