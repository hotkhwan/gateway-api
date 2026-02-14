// internal/services/thirdsvc/serviceAccount.go
package thirdsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"klynx/internal/logger"
	"klynx/models/thirdmod"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

var httpClient = &http.Client{
	Timeout:   8 * time.Second,
	Transport: otelhttp.NewTransport(http.DefaultTransport),
}

// CreateServiceAccountClient สร้าง Keycloak client แบบ service-account เปิดใช้ client-secret + ปิด web flows
// หมายเหตุ: ใช้สิทธิ admin ของระบบ (KEYCLOAK_ADMIN_* env) เพื่อเรียก Admin API
func CreateServiceAccountClient(ctx context.Context, in thirdmod.CreateServiceAccountInput) (thirdmod.CreateServiceAccountOutput, error) {
	tracer := otel.Tracer("klynx/thirdsvc")
	ctx, span := tracer.Start(ctx, "ThirdSVC.CreateServiceAccountClient")
	defer span.End()

	log := logger.FromCtx(ctx, "thirdsvc", "CreateServiceAccountClient")

	if in.ClientID == "" {
		return thirdmod.CreateServiceAccountOutput{}, errors.New("client_id is required")
	}

	kcBase := strings.TrimSuffix(os.Getenv("KEYCLOAK_URL"), "/")
	realm := os.Getenv("KEYCLOAK_REALM")
	adminUser := os.Getenv("KEYCLOAK_ADMIN_USER")
	adminPass := os.Getenv("KEYCLOAK_ADMIN_PASSWORD")

	if kcBase == "" || realm == "" || adminUser == "" || adminPass == "" {
		return thirdmod.CreateServiceAccountOutput{}, errors.New("missing KEYCLOAK_URL/REALM/ADMIN_USER/ADMIN_PASSWORD env")
	}

	// 1) ขอ admin token
	adminToken, err := getAdminToken(ctx, kcBase, adminUser, adminPass)
	if err != nil {
		return thirdmod.CreateServiceAccountOutput{}, fmt.Errorf("admin token error: %w", err)
	}

	// 2) สร้าง client
	createBody := map[string]any{
		"clientId":                  in.ClientID,
		"name":                      in.ClientID,
		"description":               in.Description,
		"serviceAccountsEnabled":    true,
		"standardFlowEnabled":       false,
		"directAccessGrantsEnabled": false,
		"publicClient":              false,
		"protocol":                  "openid-connect",
		"clientAuthenticatorType":   "client-secret",
		"fullScopeAllowed":          false,
	}

	createURL := fmt.Sprintf("%s/admin/realms/%s/clients", kcBase, realm)
	if err := kcPostJSON(ctx, createURL, adminToken, createBody, http.StatusCreated, http.StatusConflict); err != nil {
		// 409 = clientId ซ้ำ -> ถือว่าเราจะไปดึง id + สร้าง secret ต่อ
		if !errors.Is(err, errKCConflict) {
			return thirdmod.CreateServiceAccountOutput{}, fmt.Errorf("create client failed: %w", err)
		}
	}

	// 3) หา internal id ของ client
	clientInternalID, err := findClientInternalID(ctx, kcBase, realm, adminToken, in.ClientID)
	if err != nil {
		return thirdmod.CreateServiceAccountOutput{}, fmt.Errorf("find client id failed: %w", err)
	}

	// 4) สร้าง/รีเจน client-secret
	secret, err := regenClientSecret(ctx, kcBase, realm, adminToken, clientInternalID)
	if err != nil {
		return thirdmod.CreateServiceAccountOutput{}, fmt.Errorf("regen secret failed: %w", err)
	}

	log.Info().
		Str("client_id", in.ClientID).
		Msg("service-account client is ready")

	return thirdmod.CreateServiceAccountOutput{
		ClientID:     in.ClientID,
		ClientSecret: secret,
	}, nil
}

// GetClientCredentialsToken เรียก token endpoint แบบ client_credentials
func GetClientCredentialsToken(ctx context.Context, clientID, clientSecret, scope string) (thirdmod.ClientCredentialsToken, error) {
	tracer := otel.Tracer("klynx/thirdsvc")
	ctx, span := tracer.Start(ctx, "ThirdSVC.GetClientCredentialsToken")
	defer span.End()

	log := logger.FromCtx(ctx, "thirdsvc", "GetClientCredentialsToken")

	kcBase := strings.TrimSuffix(os.Getenv("KEYCLOAK_URL"), "/")
	realm := os.Getenv("KEYCLOAK_REALM")
	if kcBase == "" || realm == "" {
		return thirdmod.ClientCredentialsToken{}, errors.New("missing KEYCLOAK_URL/REALM env")
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	if scope != "" {
		form.Set("scope", scope)
	}

	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", kcBase, realm)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return thirdmod.ClientCredentialsToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return thirdmod.ClientCredentialsToken{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status", resp.StatusCode).Str("client_id", clientID).Msg("client_credentials http error")
		return thirdmod.ClientCredentialsToken{}, errors.New("unauthorized")
	}

	var out thirdmod.ClientCredentialsToken
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return thirdmod.ClientCredentialsToken{}, err
	}
	return out, nil
}

// -------------------- Helpers (Keycloak Admin) --------------------

var errKCConflict = errors.New("kc_conflict")

func getAdminToken(ctx context.Context, kcBase, user, pass string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", "admin-cli")
	form.Set("username", user)
	form.Set("password", pass)

	url := kcBase + "/realms/master/protocol/openid-connect/token"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("get admin token failed")
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

func kcPostJSON(ctx context.Context, url, bearer string, body any, expected int, alsoOK ...int) error {
	bts, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(bts)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == expected {
		return nil
	}
	for _, code := range alsoOK {
		if resp.StatusCode == code {
			return errKCConflict
		}
	}
	return fmt.Errorf("kc http %d", resp.StatusCode)
}

func findClientInternalID(ctx context.Context, kcBase, realm, bearer, clientID string) (string, error) {
	// GET /admin/realms/{realm}/clients?clientId={clientID}
	u := fmt.Sprintf("%s/admin/realms/%s/clients?clientId=%s", kcBase, realm, url.QueryEscape(clientID))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("list clients failed")
	}
	var arr []struct {
		ID       string `json:"id"`
		ClientID string `json:"clientId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil {
		return "", err
	}
	if len(arr) == 0 || arr[0].ID == "" {
		return "", errors.New("client not found after create")
	}
	return arr[0].ID, nil
}

func regenClientSecret(ctx context.Context, kcBase, realm, bearer, internalID string) (string, error) {
	// POST /admin/realms/{realm}/clients/{id}/client-secret
	u := fmt.Sprintf("%s/admin/realms/%s/clients/%s/client-secret", kcBase, realm, internalID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 200 OK จะมี {"type":"secret","value":"..."}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("regen secret http %d", resp.StatusCode)
	}
	var out struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Value == "" {
		return "", errors.New("empty client secret")
	}
	return out.Value, nil
}
