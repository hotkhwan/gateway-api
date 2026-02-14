// internal/services/authzsvc/permissionSubjectCheck.go
package authzsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"klynx/config"
	"klynx/models/authzmod"
	"klynx/utils/traceutil"
)

func PermissionSubjectCheck(ctx context.Context, permRequest authzmod.PermissionSubjectCheckRequest) ([]authzmod.PermissionSubjectCheckItem, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/authzsvc",
		"authorization.PermissionSubjectCheck",
		"authzsvc", "PermissionSubjectCheck",
	)
	defer end()

	// กำหนด Target ที่เราต้องการกรองไว้ที่นี่
	targetPermissions := []string{"view", "create", "update", "delete"}

	url := fmt.Sprintf("%s/v1/tenants/%s/permissions/subject-permission", config.PermifyBaseURL, config.PermifyTenantID)

	// ข้อมูลที่ส่งไปตรวจสอบ
	payload := map[string]interface{}{
		"entity": map[string]string{
			"type": permRequest.Entity.Type,
			"id":   permRequest.Entity.ID,
		},
		"subject": map[string]string{
			"type": permRequest.Subject.Type,
			"id":   permRequest.Subject.ID,
		},
		"permissions": targetPermissions,
		"metadata": map[string]interface{}{
			"depth":        20,
			"only_allowed": false,
		},
	}

	//log.Info().Interface("payload", payload).Msg("✅ Permission check payload")

	data, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("permify error: status=%d body=%s", resp.StatusCode, string(body))
	}

	//log.Info().Interface("body", body).Msg("✅ Permission check response")

	var result struct {
		Results map[string]string `json:"results"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	var returnPermissions []authzmod.PermissionSubjectCheckItem

	// นำ Map ที่ Permify ตอบกลับมา (result.Results)
	item := authzmod.PermissionSubjectCheckItem{
		Create: result.Results["create"] == "CHECK_RESULT_ALLOWED",
		View:   result.Results["view"] == "CHECK_RESULT_ALLOWED",
		Update: result.Results["update"] == "CHECK_RESULT_ALLOWED",
		Delete: result.Results["delete"] == "CHECK_RESULT_ALLOWED",
	}

	// Append เข้าไปใน Slice (ให้ได้ format [ {} ] ตามที่ต้องการ)
	returnPermissions = append(returnPermissions, item)

	log.Info().Interface("permissions", returnPermissions).Msg("✅ Subject permissions retrieveds")
	return returnPermissions, nil
}
