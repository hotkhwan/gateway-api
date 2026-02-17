// internal/gateways/authzgw/permifyRestDebug.go
package authzgw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hotkhwan/gateway-api/config"
)

type permifyEntity struct {
	Type string `json:"type"`
	Id   string `json:"id"`
}

type permifySubject struct {
	Type string `json:"type"`
	Id   string `json:"id"`
}

type permifyTuple struct {
	Entity   permifyEntity  `json:"entity"`
	Relation string         `json:"relation"`
	Subject  permifySubject `json:"subject"`
}

type ReadTuplesRequest struct {
	EntityType      string
	EntityId        string
	Relation        string
	SubjectType     string
	SubjectId       string
	PageSize        int
	ContinuousToken string

	SchemaVersion string
	SnapToken     string
	Depth         int
}

type ReadTuplesResponse struct {
	Tuples          []permifyTuple `json:"tuples"`
	ContinuousToken string        `json:"continuous_token"`
}

type DeleteTuplesRequest struct {
	// ใช้ลบแบบ precise filter (ปลอดภัยสุด)
	EntityType  string
	EntityId    string
	Relation    string
	SubjectType string
	SubjectId   string

	SchemaVersion string
	SnapToken     string
	Depth         int
}

type permifyRestDebugClient struct {
	httpClient *http.Client
	baseURL    string
}

func NewPermifyRestDebugClient() *permifyRestDebugClient {
	return &permifyRestDebugClient{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		baseURL:    config.PermifyBaseURL,
	}
}

func (c *permifyRestDebugClient) ReadTuples(ctx context.Context, tenantId string, req ReadTuplesRequest) (*ReadTuplesResponse, error) {
	url := fmt.Sprintf("%s/v1/tenants/%s/data/relationships/read", c.baseURL, tenantId)

	schemaVersion := req.SchemaVersion
	if schemaVersion == "" {
		schemaVersion = "latest"
	}

	depth := req.Depth
	if depth <= 0 {
		depth = 50
	}

	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	// ✅ snake_case ตาม REST ของ Permify
	payload := map[string]any{
		"metadata": map[string]any{
			"schema_version": schemaVersion,
			"depth":          depth,
		},
		"page_size": pageSize,
	}

	if req.SnapToken != "" {
		payload["metadata"].(map[string]any)["snap_token"] = req.SnapToken
	}

	if req.ContinuousToken != "" {
		payload["continuous_token"] = req.ContinuousToken
	}

	filter := map[string]any{}

	if req.EntityType != "" || req.EntityId != "" {
		entity := map[string]any{}
		if req.EntityType != "" {
			entity["type"] = req.EntityType
		}
		if req.EntityId != "" {
			entity["id"] = req.EntityId
		}
		filter["entity"] = entity
	}

	if req.Relation != "" {
		filter["relation"] = req.Relation
	}

	if req.SubjectType != "" || req.SubjectId != "" {
		subject := map[string]any{}
		if req.SubjectType != "" {
			subject["type"] = req.SubjectType
		}
		if req.SubjectId != "" {
			subject["id"] = req.SubjectId
		}
		filter["subject"] = subject
	}

	if len(filter) > 0 {
		payload["filter"] = filter
	}

	b, _ := json.Marshal(payload)

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("permify read tuples failed status=%d body=%s", resp.StatusCode, string(body))
	}

	var out ReadTuplesResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("permify read tuples decode failed: %w body=%s", err, string(body))
	}

	return &out, nil
}

func (c *permifyRestDebugClient) DeleteTuples(ctx context.Context, tenantId string, req DeleteTuplesRequest) error {
	url := fmt.Sprintf("%s/v1/tenants/%s/data/delete", c.baseURL, tenantId)

	schemaVersion := req.SchemaVersion
	if schemaVersion == "" {
		schemaVersion = "latest"
	}

	depth := req.Depth
	if depth <= 0 {
		depth = 50
	}

	// ✅ snake_case: tuple_filter
	tupleFilter := map[string]any{}

	if req.EntityType == "" {
		return fmt.Errorf("permify delete tuples requires entityType (guard)")
	}

	entity := map[string]any{
		"type": req.EntityType,
	}
	// บางเวอร์ชัน require entity.id ด้วย -> ถ้ามีให้ใส่
	if req.EntityId != "" {
		entity["id"] = req.EntityId
	}
	tupleFilter["entity"] = entity

	if req.Relation != "" {
		tupleFilter["relation"] = req.Relation
	}

	if req.SubjectType != "" || req.SubjectId != "" {
		subject := map[string]any{}
		if req.SubjectType != "" {
			subject["type"] = req.SubjectType
		}
		if req.SubjectId != "" {
			subject["id"] = req.SubjectId
		}
		tupleFilter["subject"] = subject
	}

	payload := map[string]any{
		"metadata": map[string]any{
			"schema_version": schemaVersion,
			"depth":          depth,
		},
		"tuple_filter": tupleFilter,
		"attribute_filter": map[string]any{}, 
	}

	if req.SnapToken != "" {
		payload["metadata"].(map[string]any)["snap_token"] = req.SnapToken
	}

	b, _ := json.Marshal(payload)

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("permify delete tuples failed status=%d body=%s", resp.StatusCode, string(body))
	}

	return nil
}
