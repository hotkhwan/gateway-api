// models/authzmod/profile.go
package authzmod

import "time"

type OwnerRef struct {
	Type string `json:"type" bson:"type"` // "user" | "group" | "role"
	ID   string `json:"id"   bson:"id"`
}

type Profile struct {
	Code        string        `json:"code" bson:"code"`
	Name        string        `json:"name" bson:"name"`
	Description string        `json:"description,omitempty" bson:"description,omitempty"`
	Items       []ProfileItem `json:"items,omitempty" bson:"items,omitempty"`

	Owner     *OwnerRef `json:"owner,omitempty" bson:"owner,omitempty"`
	Tags      []string  `json:"tags,omitempty"  bson:"tags,omitempty"`
	IsDeleted bool      `bson:"isDeleted"`
	CreatedAt time.Time `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
}

type ProfileItem struct {
	ID       string `json:"id" bson:"id"`
	Action   string `json:"action" bson:"action"`
	Resource string `json:"resource" bson:"resource"`
	Subject  string `json:"subject" bson:"subject"`
}

type ProfileVersion struct {
	ProfileCode string        `json:"profileCode" bson:"profileCode"`
	Version     int           `json:"version" bson:"version"`
	Items       []ProfileItem `json:"items" bson:"items"`
	Note        string        `json:"note,omitempty" bson:"note,omitempty"`
	CreatedAt   time.Time     `json:"createdAt" bson:"createdAt"`
}

/* Pagination response สำหรับ list */
type Pagination struct {
	Page         int    `json:"page"`
	PerPages     int    `json:"perPages"`
	TotalRecords int64  `json:"totalRecords"`
	TotalPages   int64  `json:"totalPages"`
	SortField    string `json:"sortField"`
	SortOrder    string `json:"sortOrder"`
}

type ProfileListResponse struct {
	Details    []*Profile `json:"details"`
	Pagination Pagination `json:"pagination"`
	Status     bool       `json:"status"`
}

type PublishItem struct {
	// รหัส resource เช่น "menu_dashboard_global", "device:xxx"
	ResourceID string `json:"resourceId"`

	// รูปแบบ subject ตรง ๆ เช่น "role:PF_OFFICE" หรือ "group:g_inno"
	// (ใช้แทน subjectType+subjectId แบบใหม่)
	Relationship string `json:"relationship,omitempty"`

	// รองรับ legacy payload แบบเก่า
	SubjectType string   `json:"subjectType,omitempty"`
	SubjectID   string   `json:"subjectId,omitempty"`
	CRUD        []string `json:"crud,omitempty"` // ["c","r","u","d"] หรือ ["read","update",...]
	Public      bool     `json:"public,omitempty"`
}

// ใช้ใน POST /authz/profiles/{code}/publish
type PublishProfileRequest struct {
	Items []PublishItem `json:"items"`
	Note  string        `json:"note,omitempty"`
}

// ใช้ใน POST /authz/profiles/{code}/plan
type PlanProfileRequest struct {
	Version int `json:"version"`
}

// ใช้ใน POST /authz/profiles/{code}/apply
// ตอนนี้ controller ยังไม่ใช้ field ใด ๆ แต่เผื่ออนาคต
type ApplyProfileRequest struct {
	Version int `json:"version,omitempty"`
}

type FactoryForceRequest struct {
	Force bool `json:"force" example:"true"`
}

// ใช้ระบุตัวตน tuple หนึ่งตัวแบบ stable key
type TupleKey struct {
	EntityType  string `bson:"entity_type"  json:"entityType"`
	EntityID    string `bson:"entity_id"    json:"entityId"`
	Relation    string `bson:"relation"     json:"relation"`
	SubjectType string `bson:"subject_type" json:"subjectType"`
	SubjectID   string `bson:"subject_id"   json:"subjectId"`
}

// เก็บ mapping ระหว่าง tuple ↔ profiles ที่ใช้อยู่
type ProfileEffectiveTuple struct {
	ID           interface{} `bson:"_id,omitempty"        json:"id,omitempty"`
	EntityType   string      `bson:"entity_type"          json:"entityType"`
	EntityID     string      `bson:"entity_id"            json:"entityId"`
	Relation     string      `bson:"relation"             json:"relation"`
	SubjectType  string      `bson:"subject_type"         json:"subjectType"`
	SubjectID    string      `bson:"subject_id"           json:"subjectId"`
	ProfileCodes []string    `bson:"profile_codes"        json:"profileCodes"`
	CreatedAt    time.Time   `bson:"created_at"           json:"createdAt"`
	UpdatedAt    time.Time   `bson:"updated_at"           json:"updatedAt"`
}
