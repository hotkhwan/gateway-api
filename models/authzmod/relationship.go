// models/authzmod/relationship.go
package authzmod

import "github.com/hotkhwan/gateway-api/models/gmod"

// RelationshipQuery สำหรับรับค่าจาก Query Parameter (Fiber)
type RelationshipQuery struct {
	SubjectType string `query:"subjectType"`
	SubjectIds  string `query:"subjectId"`  // comma-separated (ตอนนี้ใช้ค่าตรง ๆ)
	EntityType  string `query:"entityType"` // ปกติใช้ "resource"
	EntityIds   string `query:"entityId"`   // comma-separated
	Page        int    `query:"page"`
	PerPages    int    `query:"perPages"`
	SortField   string `query:"sortField"`
	SortOrder   string `query:"sortOrder"` // "asc" | "desc"
}

type RelationshipEntityFilter struct {
	Type string   `json:"type"`
	IDs  []string `json:"ids"`
}

type RelationshipReadFilter struct {
	Subject *RelationshipEntityFilter `json:"subject,omitempty"`
	Entity  *RelationshipEntityFilter `json:"entity,omitempty"`
}

type RelationshipReadMetadata struct {
	SchemaVersion string `json:"schema_version,omitempty"`
	SnapToken     string `json:"snap_token,omitempty"`
}

type RelationshipReadRequest struct {
	Metadata        *RelationshipReadMetadata `json:"metadata,omitempty"`
	Filter          RelationshipReadFilter    `json:"filter"`
	PageSize        int32                     `json:"page_size,omitempty"`
	ContinuousToken string                    `json:"continuous_token,omitempty"`
}

// Response จาก Permify
type PermifyTuple struct {
	Entity struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"entity"`
	Relation string `json:"relation"`
	Subject  struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"subject"`
}

type RelationshipReadResponse struct {
	Details    []PermifyTuple   `json:"tuples"`
	Pagination *gmod.Pagination `json:"pagination,omitempty"`
	Status     bool             `json:"status"`
}

// ✅ RevokeResourceRequest: ใช้ revoke สิทธิ์บน resource
type RevokeResourceRequest struct {
	EntityType string   `json:"entityType"`
	EntityIDs  []string `json:"entityIds"`
}
