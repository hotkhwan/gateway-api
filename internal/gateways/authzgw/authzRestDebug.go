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

// ---------- Read ----------
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
	ContinuousToken string         `json:"continuous_token"`
}

// ---------- Delete (relationship) ----------
type DeleteRelationship struct {
	Entity   permifyEntity  `json:"entity"`
	Relation string         `json:"relation"`
	Subject  permifySubject `json:"subject"`
}

// ---------- Delete (tuple) legacy ----------
type DeleteTuplesRequest struct {
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
		httpClient: &http.Client{Timeout: 25 * time.Second},
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

// ✅ NEW: delete relationships (the safe way, no attribute_filter)
func (c *permifyRestDebugClient) DeleteRelationships(ctx context.Context, tenantId string, schemaVersion string, depth int, rels []DeleteRelationship) error {
	url := fmt.Sprintf("%s/v1/tenants/%s/data/relationships/delete", c.baseURL, tenantId)

	if schemaVersion == "" {
		schemaVersion = "latest"
	}
	if depth <= 0 {
		depth = 50
	}

	payload := map[string]any{
		"metadata": map[string]any{
			"schema_version": schemaVersion,
			"depth":          depth,
		},
		"relationships": rels,
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
		return fmt.Errorf("permify relationships delete failed status=%d body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// ✅ DEBUG ONLY: delete all tuples under organization entityId by read+delete
func (c *permifyRestDebugClient) DeleteOrgTuples(ctx context.Context, tenantId string, orgId string) error {
	if tenantId == "" || orgId == "" {
		return fmt.Errorf("tenantId and orgId are required")
	}

	continuousToken := ""
	for {
		readRes, err := c.ReadTuples(ctx, tenantId, ReadTuplesRequest{
			EntityType:      "organization",
			EntityId:        orgId,
			PageSize:        200,
			ContinuousToken: continuousToken,
			SchemaVersion:   "latest",
			Depth:           50,
		})
		if err != nil {
			return err
		}
		if len(readRes.Tuples) == 0 {
			break
		}

		rels := make([]DeleteRelationship, 0, len(readRes.Tuples))
		for _, t := range readRes.Tuples {
			rels = append(rels, DeleteRelationship{
				Entity:   permifyEntity{Type: t.Entity.Type, Id: t.Entity.Id},
				Relation: t.Relation,
				Subject:  permifySubject{Type: t.Subject.Type, Id: t.Subject.Id},
			})
		}

		// batch delete (faster than per-tuple)
		if err := c.DeleteRelationships(ctx, tenantId, "latest", 50, rels); err != nil {
			return err
		}

		if readRes.ContinuousToken == "" || readRes.ContinuousToken == continuousToken {
			break
		}
		continuousToken = readRes.ContinuousToken
	}

	return nil
}

// ⚠️ Legacy: keep but DO NOT send attribute_filter at all (empty object breaks some servers)
func (c *permifyRestDebugClient) DeleteTuples(ctx context.Context, tenantId string, req DeleteTuplesRequest) error {
	url := fmt.Sprintf("%s/v1/tenants/%s/data/delete", c.baseURL, tenantId)

	if req.EntityType == "" {
		return fmt.Errorf("permify delete tuples requires entityType")
	}

	schemaVersion := req.SchemaVersion
	if schemaVersion == "" {
		schemaVersion = "latest"
	}

	depth := req.Depth
	if depth <= 0 {
		depth = 50
	}

	tupleFilter := map[string]any{
		"entity": map[string]any{"type": req.EntityType},
	}
	if req.EntityId != "" {
		tupleFilter["entity"].(map[string]any)["id"] = req.EntityId
	}
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

	// ✅ Delete for ALL
	payload := map[string]any{
		"metadata": map[string]any{
			"schema_version": schemaVersion,
			"depth":          depth,
		},
		"tuple_filter":     tupleFilter,
		"attribute_filter": map[string]any{},
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
