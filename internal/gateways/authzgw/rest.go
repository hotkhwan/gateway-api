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
)

type RestClient struct {
	baseURL string
	httpc   *http.Client
}

func NewRestClient() *RestClient {
	base := strings.TrimRight(config.PermifyBaseURL, "/")
	if base == "" {
		return nil
	}
	return &RestClient{
		baseURL: base,
		httpc:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (r *RestClient) WriteTuples(ctx context.Context, tenantId string, tuples []map[string]interface{}) error {
	if r == nil {
		return fmt.Errorf("authzgw rest client is nil")
	}
	if tenantId == "" {
		return fmt.Errorf("tenantId required")
	}
	if len(tuples) == 0 {
		return nil
	}

	meta := map[string]interface{}{}
	if config.CurrentSchemaVersion != "" {
		meta["schema_version"] = config.CurrentSchemaVersion
	}

	payload := map[string]interface{}{
		"metadata": meta,   // ✅ ต้องมีแม้ว่าง
		"tuples":   tuples, // ✅ batch
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

func (r *RestClient) LookupOrganizations(
	ctx context.Context,
	tenantId string,
	userId string,
) ([]string, error) {

	url := fmt.Sprintf(
		"%s/v1/tenants/%s/permissions/lookup-entity",
		r.baseURL,
		tenantId,
	)

	payload := map[string]interface{}{
		"entity_type": "organization",
		"permission":  "view",
		"subject": map[string]string{
			"type": "user",
			"id":   userId,
		},
	}

	data, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewReader(data),
	)

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

	var ids []string
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

	url := fmt.Sprintf(
		"%s/v1/tenants/%s/permissions/check",
		r.baseURL,
		tenantId,
	)

	payload := map[string]interface{}{
		"entity": map[string]string{
			"type": entityType,
			"id":   entityId,
		},
		"permission": permission,
		"subject": map[string]string{
			"type": subjectType,
			"id":   subjectId,
		},
	}

	data, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewReader(data),
	)

	req.Header.Set("Content-Type", "application/json")

	res, err := r.httpc.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		return false, fmt.Errorf("authz check failed")
	}

	var resp struct {
		Can string `json:"can"`
	}

	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return false, err
	}

	return resp.Can == "RESULT_ALLOWED", nil
}
