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
		return ErrNilRestClient
	}
	if strings.TrimSpace(r.baseURL) == "" {
		return ErrEmptyPermifyBaseURL
	}
	if r.httpc == nil {
		r.httpc = &http.Client{Timeout: 10 * time.Second}
	}
	return nil
}
func normalizeSubjectId(subjectId string) string {
	return strings.TrimSpace(subjectId)
}

// func normalizeSubjectId(subjectType string, subjectId string) string {
// 	subjectType = strings.TrimSpace(subjectType)
// 	subjectId = strings.TrimSpace(subjectId)
// 	if subjectId == "" {
// 		return ""
// 	}

// 	// Permify convention in your codebase: "user:<uuid>"
// 	if subjectType == "user" && !strings.HasPrefix(subjectId, "user:") {
// 		return "user:" + subjectId
// 	}

// 	return subjectId
// }

func (r *RestClient) WriteTuples(ctx context.Context, tenantId string, tuples []map[string]any) error {
	log := logger.FromCtx(ctx, "authzgw", "WriteTuples")
	if err := r.ensure(); err != nil {
		return err
	}
	tenantId = strings.TrimSpace(tenantId)
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
	log.Info().
		Str("tenantId", tenantId).
		Str("schemaVersion", config.CurrentSchemaVersion).
		Int("tupleCount", len(tuples)).
		Msg("🚀 writing tuples to permify")
	return nil
}

func (r *RestClient) DeleteOrgTuples(ctx context.Context, tenantId string, orgId string) error {
	if err := r.ensure(); err != nil {
		return err
	}
	tenantId = strings.TrimSpace(tenantId)
	orgId = strings.TrimSpace(orgId)
	if tenantId == "" || orgId == "" {
		return fmt.Errorf("tenantId and orgId required")
	}

	schemaVersion := config.CurrentSchemaVersion
	if schemaVersion == "" {
		return fmt.Errorf("current schema version is empty")
	}

	url := fmt.Sprintf("%s/v1/tenants/%s/data/delete", r.baseURL, tenantId)

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
	}

	log := logger.FromCtx(ctx, "authzgw", "DeleteOrgTuples")
	data, _ := json.Marshal(payload)
	log.Info().Str("tenantId", tenantId).Str("orgId", orgId).RawJSON("payload", data).Msg("permify data/delete request")

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
	tenantId = strings.TrimSpace(tenantId)
	orgId = strings.TrimSpace(orgId)
	if tenantId == "" || orgId == "" {
		return fmt.Errorf("tenantId and orgId required")
	}

	url := fmt.Sprintf("%s/v1/tenants/%s/relationships/delete", r.baseURL, tenantId)

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
	log.Info().Str("tenantId", tenantId).Str("orgId", orgId).RawJSON("payload", data).Msg("permify relationships/delete request")

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
	tenantId = strings.TrimSpace(tenantId)
	userId = strings.TrimSpace(userId)
	if tenantId == "" || userId == "" {
		return nil, ErrInvalidArgs
	}

	schemaVersion := strings.TrimSpace(config.CurrentSchemaVersion)
	if schemaVersion == "" {
		return nil, fmt.Errorf("current schema version is empty")
	}

	subjectId := normalizeSubjectId(userId)
	// subjectId := normalizeSubjectId("user", userId)

	url := fmt.Sprintf("%s/v1/tenants/%s/permissions/lookup-entity", r.baseURL, tenantId)

	payload := map[string]any{
		"metadata": map[string]any{
			"schema_version": schemaVersion,
			"depth":          50,
		},
		"entity_type": "organization",
		"permission":  "view",
		"subject": map[string]any{
			"type": "user",
			"id":   subjectId, // ✅ must be user:<uuid>
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
		if strings.TrimSpace(e.Id) != "" {
			ids = append(ids, e.Id)
		}
	}

	return ids, nil
}

// Backward compatible helper (uses config.CurrentSchemaVersion)
func (r *RestClient) CheckPermission(
	ctx context.Context,
	tenantId string,
	entityType string,
	entityId string,
	permission string,
	subjectType string,
	subjectId string,
) (bool, error) {
	schemaVersion := strings.TrimSpace(config.CurrentSchemaVersion)
	if schemaVersion == "" {
		return false, fmt.Errorf("current schema version is empty")
	}
	return r.CheckPermissionWithSchemaVersion(ctx, tenantId, schemaVersion, entityType, entityId, permission, subjectType, subjectId)
}

func (r *RestClient) CheckPermissionWithSchemaVersion(
	ctx context.Context,
	tenantId string,
	schemaVersion string,
	entityType string,
	entityId string,
	permission string,
	subjectType string,
	subjectId string,
) (bool, error) {
	if err := r.ensure(); err != nil {
		return false, err
	}

	tenantId = strings.TrimSpace(tenantId)
	schemaVersion = strings.TrimSpace(schemaVersion)
	entityType = strings.TrimSpace(entityType)
	entityId = strings.TrimSpace(entityId)
	permission = strings.TrimSpace(permission)
	subjectType = strings.TrimSpace(subjectType)
	subjectId = normalizeSubjectId(subjectId)
	// subjectId = normalizeSubjectId(subjectType, subjectId)

	if tenantId == "" || schemaVersion == "" || entityType == "" || entityId == "" || permission == "" || subjectType == "" || subjectId == "" {
		return false, ErrInvalidArgs
	}

	url := fmt.Sprintf("%s/v1/tenants/%s/permissions/check", r.baseURL, tenantId)

	payload := map[string]any{
		"metadata": map[string]any{
			"schema_version": schemaVersion,
			"depth":          50,
		},
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

	body, _ := io.ReadAll(res.Body)

	if res.StatusCode >= 400 {
		return false, fmt.Errorf("authz check failed: status=%d resp=%s", res.StatusCode, string(body))
	}

	// Support both:
	// - {"can":"RESULT_ALLOWED"}
	// - {"can":true}
	var resp1 struct {
		Can string `json:"can"`
	}
	if err := json.Unmarshal(body, &resp1); err == nil && resp1.Can != "" {
		return resp1.Can == "RESULT_ALLOWED", nil
	}

	var resp2 struct {
		Can bool `json:"can"`
	}
	if err := json.Unmarshal(body, &resp2); err == nil {
		return resp2.Can, nil
	}

	return false, fmt.Errorf("authz check decode failed: %s", string(body))
}

func (r *RestClient) DeleteEntityRelationships(
	ctx context.Context,
	tenantId string,
	entityType string,
	entityId string,
) error {

	if err := r.ensure(); err != nil {
		return err
	}

	tenantId = strings.TrimSpace(tenantId)
	entityType = strings.TrimSpace(entityType)
	entityId = strings.TrimSpace(entityId)

	if tenantId == "" || entityType == "" || entityId == "" {
		return fmt.Errorf("invalid delete entity relationships args")
	}

	url := fmt.Sprintf("%s/v1/tenants/%s/relationships/delete", r.baseURL, tenantId)

	payload := map[string]any{
		"filter": map[string]any{
			"entity": map[string]any{
				"type": entityType,
				"ids":  []string{entityId},
			},
		},
	}

	data, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	res, err := r.httpc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("permify delete relationships failed: %s", string(body))
	}

	return nil
}

func (r *RestClient) ListEntityRelationships(
	ctx context.Context,
	tenantId string,
	entityType string,
	entityId string,
) ([]Relationship, error) {

	if err := r.ensure(); err != nil {
		return nil, err
	}

	// ✅ FIX: ใช้ endpoint เดียวกับ permifyRestDebugClient
	url := fmt.Sprintf("%s/v1/tenants/%s/data/relationships/read", r.baseURL, tenantId)

	payload := map[string]any{
		"metadata": map[string]any{
			"schema_version": config.CurrentSchemaVersion,
			"depth":          50,
		},
		"page_size": 200,
		// ✅ FIX: filter format ถูกต้อง
		"filter": map[string]any{
			"entity": map[string]any{
				"type": entityType,
				"id":   entityId,
			},
		},
	}

	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		// ✅ FIX: code 5 = Not Found = ไม่มี tuple = empty list
		var permifyErr struct {
			Code int `json:"code"`
		}
		if jsonErr := json.Unmarshal(b, &permifyErr); jsonErr == nil && permifyErr.Code == 5 {
			return []Relationship{}, nil
		}
		return nil, fmt.Errorf("permify read failed: %s", string(b))
	}

	var result struct {
		Tuples []struct {
			Entity struct {
				Type string `json:"type"`
				Id   string `json:"id"`
			} `json:"entity"`
			Relation string `json:"relation"`
			Subject  struct {
				Type string `json:"type"`
				Id   string `json:"id"`
			} `json:"subject"`
		} `json:"tuples"`
	}

	if err := json.Unmarshal(b, &result); err != nil {
		return nil, err
	}

	out := make([]Relationship, 0, len(result.Tuples))
	for _, t := range result.Tuples {
		out = append(out, Relationship{
			Entity:   EntityRef{Type: t.Entity.Type, ID: t.Entity.Id},
			Relation: t.Relation,
			Subject:  SubjectRef{Type: t.Subject.Type, ID: t.Subject.Id},
		})
	}

	return out, nil
}

func (r *RestClient) ListRelationshipsBySubject(
	ctx context.Context,
	tenantId string,
	subjectType string,
	subjectId string,
) ([]Relationship, error) {
	if err := r.ensure(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v1/tenants/%s/data/relationships/read", r.baseURL, tenantId)

	payload := map[string]any{
		"metadata": map[string]any{
			"schema_version": config.CurrentSchemaVersion,
			"depth":          50,
		},
		"page_size": 200,
		"filter": map[string]any{
			"subject": map[string]any{
				"type": subjectType,
				"ids":  []string{subjectId},
			},
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var permifyErr struct {
			Code int `json:"code"`
		}
		if jsonErr := json.Unmarshal(b, &permifyErr); jsonErr == nil && permifyErr.Code == 5 {
			return []Relationship{}, nil
		}
		return nil, fmt.Errorf("permify read by subject failed: %s", string(b))
	}

	var result struct {
		Tuples []struct {
			Entity struct {
				Type string `json:"type"`
				Id   string `json:"id"`
			} `json:"entity"`
			Relation string `json:"relation"`
			Subject  struct {
				Type string `json:"type"`
				Id   string `json:"id"`
			} `json:"subject"`
		} `json:"tuples"`
	}

	if err := json.Unmarshal(b, &result); err != nil {
		return nil, err
	}

	out := make([]Relationship, 0, len(result.Tuples))
	for _, t := range result.Tuples {
		out = append(out, Relationship{
			Entity:   EntityRef{Type: t.Entity.Type, ID: t.Entity.Id},
			Relation: t.Relation,
			Subject:  SubjectRef{Type: t.Subject.Type, ID: t.Subject.Id},
		})
	}
	return out, nil
}

func (r *RestClient) DeleteSpecificTupleWithRelation(
	ctx context.Context,
	tenantId, entityType, entityId, relation, subjectType, subjectId string,
) error {
	if err := r.ensure(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/v1/tenants/%s/data/delete", r.baseURL, tenantId)

	payload := map[string]any{
		"tuple_filter": map[string]any{
			"entity": map[string]any{
				"type": entityType,
				"ids":  []string{entityId},
			},
			// ❌ ไม่ใส่ relation — ให้ match ด้วย subject เท่านั้น
			"subject": map[string]any{
				"type":     subjectType,
				"ids":      []string{subjectId}, // ✅ ลอง "ids" แทน "id"
				"relation": "",
			},
		},
		"attribute_filter": map[string]any{},
	}

	data, _ := json.Marshal(payload)

	// ✅ log payload ดูว่าส่งอะไรไปจริงๆ
	log := logger.FromCtx(ctx, "authzgw", "DeleteSpecificTupleWithRelation")
	log.Info().
		Str("url", url).
		RawJSON("payload", data).
		Msg("permify delete specific tuple")

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	log.Info().
		Int("status", resp.StatusCode).
		RawJSON("response", body).
		Msg("permify delete response")

	if resp.StatusCode >= 400 {
		return fmt.Errorf("permify delete tuple failed: %s", string(body))
	}

	return nil
}
