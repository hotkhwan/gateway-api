// internal/gateways/authzgw/rest.go
package authzgw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
)

type RestClient struct {
	baseURL string
	httpc   *http.Client
}

func NewRestClient() *RestClient {
	base := strings.TrimRight(config.PermifyBaseURL, "/")
	return &RestClient{
		baseURL: base,
		httpc:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (r *RestClient) ensure() error {
	if r == nil {
		return fmt.Errorf("authzgw rest client is nil")
	}
	if r.baseURL == "" {
		return fmt.Errorf("permify baseURL is empty")
	}
	return nil
}

func (r *RestClient) WriteTuples(ctx context.Context, tenantId string, tuples []map[string]any) error {
	if err := r.ensure(); err != nil {
		return err
	}
	if tenantId == "" {
		return fmt.Errorf("tenantId required")
	}
	if len(tuples) == 0 {
		return nil
	}

	meta := map[string]any{}
	if config.CurrentSchemaVersion != "" {
		meta["schema_version"] = config.CurrentSchemaVersion
	}

	payload := map[string]any{
		"metadata": meta,
		"tuples":   tuples,
	}

	data, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/v1/tenants/%s/data/write", r.baseURL, tenantId)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	res, err := r.httpc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("authzgw write failed: status=%d resp=%s", res.StatusCode, string(body))
	}

	return nil
}

// DeleteOrgTuples ลบทุก tuple ที่ผูกกับ org นี้ (entity=organization:id=orgId)
// NOTE: Permify REST requires attribute_filter even if empty
func (r *RestClient) DeleteOrgTuples(ctx context.Context, tenantId string, orgId string) error {
  if err := r.ensure(); err != nil {
    return err
  }
  if tenantId == "" {
    return fmt.Errorf("tenantId required")
  }
  if orgId == "" {
    return fmt.Errorf("orgId required")
  }

  schemaVersion := config.CurrentSchemaVersion
  if schemaVersion == "" {
    schemaVersion = "latest"
  }

  url := fmt.Sprintf("%s/v1/tenants/%s/data/delete", r.baseURL, tenantId)

  // ✅ STOP CASCADE: depth = 0 (หรือไม่ส่ง depth เลยก็ได้)
  payload := map[string]any{
    "metadata": map[string]any{
      "schema_version": schemaVersion,
      "depth":          0,
    },
    "tuple_filter": map[string]any{
      "entity": map[string]any{
        "type": "organization",
        "id":   orgId,
      },
    },
    // ❌ อย่าส่ง attribute_filter เลย (ไม่จำเป็น และลดความเสี่ยง)
    // "attribute_filter": map[string]any{},
  }

  log := logger.FromCtx(ctx, "authzgw", "DeleteOrgTuples")
  data, _ := json.Marshal(payload)
  log.Info().
    Str("tenantId", tenantId).
    Str("orgId", orgId).
    RawJSON("payload", data).
    Msg("permify data/delete request")

  req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
  req.Header.Set("Content-Type", "application/json")

  res, err := r.httpc.Do(req)
  if err != nil {
    return err
  }
  defer res.Body.Close()

  body, _ := io.ReadAll(res.Body)
  if res.StatusCode >= http.StatusBadRequest {
    return fmt.Errorf("authzgw delete org tuples failed: status=%d resp=%s", res.StatusCode, string(body))
  }
  return nil
}

func (r *RestClient) DeleteOrgRelationships(ctx context.Context, tenantId string, orgId string) error {
  if err := r.ensure(); err != nil {
    return err
  }
  if tenantId == "" {
    return fmt.Errorf("tenantId required")
  }
  if orgId == "" {
    return fmt.Errorf("orgId required")
  }

  url := fmt.Sprintf("%s/v1/tenants/%s/relationships/delete", r.baseURL, tenantId)

  // ✅ CRITICAL: ids (array) not id
  payload := map[string]any{
    "filter": map[string]any{
      "entity": map[string]any{
        "type": "organization",
        "ids":  []string{orgId},
      },
    },
  }

  log := logger.FromCtx(ctx, "authzgw", "DeleteOrgRelationships")
  data, _ := json.Marshal(payload)
  log.Info().
    Str("tenantId", tenantId).
    Str("orgId", orgId).
    RawJSON("payload", data).
    Msg("permify relationships/delete request")

  req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
  req.Header.Set("Content-Type", "application/json")

  res, err := r.httpc.Do(req)
  if err != nil {
    return err
  }
  defer res.Body.Close()

  body, _ := io.ReadAll(res.Body)
  if res.StatusCode >= http.StatusBadRequest {
    return fmt.Errorf("authzgw delete org relationships failed: status=%d resp=%s", res.StatusCode, string(body))
  }

  return nil
}

func (r *RestClient) LookupOrganizations(ctx context.Context, tenantId string, userId string) ([]string, error) {
	if err := r.ensure(); err != nil {
		return nil, err
	}
	if tenantId == "" {
		return nil, fmt.Errorf("tenantId required")
	}
	if userId == "" {
		return nil, fmt.Errorf("userId required")
	}

	url := fmt.Sprintf("%s/v1/tenants/%s/permissions/lookup-entity", r.baseURL, tenantId)

	payload := map[string]any{
		"entity_type": "organization",
		"permission":  "view",
		"subject": map[string]any{
			"type": "user",
			"id":   userId,
		},
	}

	data, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	res, err := r.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("lookup failed: %s", string(body))
	}

	var resp struct {
		Entities []struct {
			Id string `json:"id"`
		} `json:"entities"`
	}

	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(resp.Entities))
	for _, e := range resp.Entities {
		ids = append(ids, e.Id)
	}

	return ids, nil
}

func (r *RestClient) CheckPermission(
	ctx context.Context,
	tenantId string,
	entityType string,
	entityId string,
	permission string,
	subjectType string,
	subjectId string,
) (bool, error) {

	if err := r.ensure(); err != nil {
		return false, err
	}

	url := fmt.Sprintf("%s/v1/tenants/%s/permissions/check", r.baseURL, tenantId)

	payload := map[string]any{
		"entity": map[string]any{
			"type": entityType,
			"id":   entityId,
		},
		"permission": permission,
		"subject": map[string]any{
			"type": subjectType,
			"id":   subjectId,
		},
	}

	data, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	res, err := r.httpc.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(res.Body)
		return false, fmt.Errorf("authz check failed: %s", string(body))
	}

	var resp struct {
		Can string `json:"can"`
	}

	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return false, err
	}

	return resp.Can == "RESULT_ALLOWED", nil
}
