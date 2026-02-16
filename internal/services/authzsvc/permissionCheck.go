// internal/services/authzsvc/permissionCheck.go
package authzsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// PermissionCheck ตรวจสอบ permission จาก Permify REST API
func PermissionCheck(ctx context.Context, depth int, entityType, entityID, permission, subjectType, subjectID string) (bool, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/authzsvc",
		"authorization.PermissionCheck",
		"authzsvc", "PermissionCheck",
	)
	defer end()

	if depth <= 0 {
		depth = 50
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// ✅ กลับมาใช้ endpoint เดิมที่เคยตอบ ERROR_CODE_SCHEMA_NOT_FOUND
	url := fmt.Sprintf("%s/v1/tenants/%s/permissions/check", config.PermifyBaseURL, config.PermifyTenantID)

	// ❗ ไม่ต้องส่ง schema_version / snap_token ไปเลย
	// ให้ Permify ใช้ schema/snapshot ล่าสุดของ tenant เอง
	metadata := map[string]interface{}{
		"depth": depth,
	}

	payload := map[string]interface{}{
		"entity": map[string]string{
			"type": entityType,
			"id":   entityID,
		},
		"permission": permission,
		"subject": map[string]string{
			"type": subjectType,
			"id":   subjectID,
		},
		"metadata": metadata,
	}

	log.Info().
		Str("subject", subjectID).
		Str("permission", permission).
		Str("entityType", entityType).
		Str("entityID", entityID).
		Msg("🔍 Checking permission (REST)")

	data, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	log.Info().
		Str("url", url).
		RawJSON("payload", data).
		Msg("📤 Sending permission check request to Permify")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error().
			Err(err).
			Str("url", url).
			RawJSON("payload", data).
			Msg("❌ REST Call Failed (network or Permify down)")
		return false, fmt.Errorf("permission check failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Info().Str("body", string(body)).Msg("📩 Permify responded to permission check")

	if resp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", resp.StatusCode).
			Str("url", url).
			RawJSON("payload", data).
			RawJSON("response", body).
			Msg("❌ Permission check failed (non-200 response)")
		return false, fmt.Errorf("permission check failed: status=%d resp=%s", resp.StatusCode, string(body))
	}

	var result struct {
		Can string `json:"can"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Error().
			Err(err).
			RawJSON("response", body).
			Msg("❌ Failed to parse Permify check response")
		return false, fmt.Errorf("permission check failed: cannot parse response")
	}

	allowed := result.Can == "CHECK_RESULT_ALLOWED"
	log.Info().
		Bool("allowed", allowed).
		Msg("✅ Permission check result (REST)")

	return allowed, nil
}
