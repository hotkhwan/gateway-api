// config/permifyRest.go
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"
)

var (
	PermifyBaseURL = fmt.Sprintf("http://%s", os.Getenv("PERMIFY_URI"))
)

func InitPermifyRest() {
	uri := os.Getenv("PERMIFY_URI")
	if uri == "" {
		uri = "authz-permify.iam.svc.cluster.local:3476" // ✅ fallback
	}
	PermifyBaseURL = fmt.Sprintf("http://%s", uri)

	if t := os.Getenv("KEYCLOAK_REALM"); t != "" {
		PermifyTenantID = t
	} else if t := os.Getenv("PERMIFY_TENANT_ID"); t != "" {
		PermifyTenantID = t
	} else {
		PermifyTenantID = "klynx"
	}

	if !CheckPermifyHealth() {
		panic("Permify not healthy")
	}
	FetchLatestSchemaVersion()
}

func getTenantID() string {
	if t := os.Getenv("KEYCLOAK_REALM"); t != "" {
		return t
	}
	if t := os.Getenv("PERMIFY_TENANT_ID"); t != "" {
		return t
	}
	return "aliza"
}

// ✅ ใช้ตรวจสอบ Health (REST)
func CheckPermifyHealth() bool {
	log := logger.Boot("permify", "CheckPermifyHealth")
	url := fmt.Sprintf("%s/healthz", PermifyBaseURL)

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		log.Error().Err(err).Str("url", url).Msg("❌ Permify health check failed")
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error().
			Int("status", resp.StatusCode).
			RawJSON("response", body).
			Msg("❌ Permify health check not OK")
		return false
	}

	log.Info().Str("url", url).Msg("✅ Permify health OK")
	return true
}

// ✅ ใช้อ่าน Schema ล่าสุด (ถ้าต้องการ)
func FetchLatestSchemaVersion() {
	log := logger.Boot("permify", "FetchLatestSchemaVersion")

	url := fmt.Sprintf("%s/v1/tenants/{{ KC_REALM }}/schemas/list?tenant_id=%s&page_size=1", PermifyBaseURL, PermifyTenantID)
	resp, err := http.Get(url)
	if err != nil {
		log.Warn().Err(err).Msg("⚠️ Failed to fetch schema list, fallback to latest")
		CurrentSchemaVersion = "latest"
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warn().Int("status", resp.StatusCode).Msg("⚠️ Schema list request not OK, fallback to latest")
		CurrentSchemaVersion = "latest"
		return
	}

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Schemas []struct {
			Version   string `json:"version"`
			CreatedAt string `json:"created_at"`
		} `json:"schemas"`
	}

	if err := json.Unmarshal(body, &result); err != nil || len(result.Schemas) == 0 {
		log.Warn().Err(err).Msg("⚠️ Failed to parse schema list, fallback to latest")
		CurrentSchemaVersion = "latest"
		return
	}

	CurrentSchemaVersion = result.Schemas[0].Version
	log.Info().
		Str("schema_version", CurrentSchemaVersion).
		Msg("✅ Latest schema version fetched successfully (REST)")
}
