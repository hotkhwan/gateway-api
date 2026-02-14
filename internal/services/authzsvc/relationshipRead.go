// internal/services/authzsvc/relationshipRead.go
package authzsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"klynx/config"
	"klynx/models/authzmod"
	"klynx/utils/traceutil"
)

// ReadRelationships ใช้ Permify REST API เพื่อดึงข้อมูลความสัมพันธ์ (raw map)
func ReadRelationships(ctx context.Context, req authzmod.RelationshipReadRequest) (map[string]interface{}, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/authzsvc",
		"authorization.ReadRelationships",
		"authzsvc", "ReadRelationships",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	data, _ := json.Marshal(req)
	url := fmt.Sprintf("%s/v1/tenants/%s/data/relationships/read", config.PermifyBaseURL, config.PermifyTenantID)

	log.Info().
		Str("url", url).
		RawJSON("payload", data).
		Msg("📥 Reading relationships from Permify")

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Msg("❌ Error sending request to Permify")
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", resp.StatusCode).
			RawJSON("response", body).
			Msg("❌ Permify read failed")
		return nil, fmt.Errorf("permify read failed: %s", string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Error().Err(err).Msg("❌ Failed to unmarshal Permify response (map)")
		return nil, err
	}
	return result, nil
}

// GetRelationships : คืนเป็น struct เพื่อใช้ใน services อื่น ๆ
func GetRelationships(ctx context.Context, req *authzmod.RelationshipReadRequest) (*authzmod.RelationshipReadResponse, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/authzsvc",
		"authorization.GetRelationships",
		"authzsvc", "GetRelationships",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	data, _ := json.Marshal(req)
	url := fmt.Sprintf("%s/v1/tenants/%s/data/relationships/read", config.PermifyBaseURL, config.PermifyTenantID)

	log.Info().
		Str("url", url).
		RawJSON("payload", data).
		Msg("📥 GetRelationships from Permify")

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Msg("❌ Error sending request to Permify")
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", resp.StatusCode).
			RawJSON("response", body).
			Msg("❌ Permify read failed")
		return nil, fmt.Errorf("permify read failed: %s", string(body))
	}

	var result authzmod.RelationshipReadResponse
	if err := json.Unmarshal(body, &result); err != nil {
		log.Error().Err(err).Msg("❌ Failed to unmarshal Permify response (struct)")
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}
	return &result, nil
}
