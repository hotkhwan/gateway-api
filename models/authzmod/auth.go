// models/authmodel/auth.go
package authzmod

type PermissionCheckRequest struct {
	Metadata struct {
		SchemaVersion string `json:"schema_version,omitempty"`
		SnapToken     string `json:"snap_token,omitempty"`
		Depth         int    `json:"depth" validate:"required"`
	} `json:"metadata" validate:"required"`
	Entity struct {
		Type string `json:"type" validate:"required"` // เช่น "resource"
		ID   string `json:"id" validate:"required"`
	} `json:"entity" validate:"required"`
	Permission string `json:"permission" validate:"required"`
	Subject    struct {
		Type string `json:"type" validate:"required"` // เช่น "user", "group", "role"
		ID   string `json:"id" validate:"required"`
	} `json:"subject" validate:"required"`
}

type PermissionMetadata struct {
	SchemaVersion string `json:"schema_version,omitempty"`
	SnapToken     string `json:"snap_token,omitempty"`
	Depth         int    `json:"depth" validate:"required"`
}

// แยก Entity ออกมา
type PermissionEntity struct {
	Type string `json:"type" validate:"required"`
	ID   string `json:"id" validate:"required"`
}

// แยก Subject ออกมา
type PermissionSubject struct {
	Type string `json:"type" validate:"required"`
	ID   string `json:"id" validate:"required"`
}

// ตัวหลักที่นำเอา Type ข้างบนมาประกอบกัน (Composition)
type PermissionSubjectCheckRequest struct {
	Metadata   PermissionMetadata `json:"metadata" validate:"required"`
	Entity     PermissionEntity   `json:"entity" validate:"required"`
	Permission []string           `json:"permission" validate:"required"`
	Subject    PermissionSubject  `json:"subject" validate:"required"`
}

type PermissionSubjectCheckResponse struct {
	Permissions []PermissionSubjectCheckItem `json:"permissions"`
}
type PermissionSubjectCheckItem struct {
	Create bool `json:"create"`
	View   bool `json:"view"`
	Update bool `json:"update"`
	Delete bool `json:"delete"`
}

// ===== Batch permission check (ใหม่ ย้ายมาไว้ที่นี่) =====

// ใช้ภายใน service เพื่อส่งเข้า batch API
type PermissionCheckInput struct {
	EntityType  string
	EntityID    string
	Permission  string
	SubjectType string
	SubjectID   string
}

// สำหรับ JSON payload ไป Permify: /permissions/check/batch

type PermissionCheckItem struct {
	Entity struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"entity"`
	Permission string `json:"permission"`
	Subject    struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"subject"`
}

type PermissionCheckBatchRequest struct {
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Checks   []PermissionCheckItem  `json:"checks"`
}

// สำหรับ decode response จาก Permify batch API
type PermissionCheckDecision struct {
	Entity struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"entity"`
	Permission string `json:"permission"`
	Subject    struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"subject"`
	// Permify ปัจจุบันใช้ "can": "CHECK_RESULT_ALLOWED" / "CHECK_RESULT_DENIED"
	Can string `json:"can"`
}

type PermissionCheckBatchResponse struct {
	Decisions []PermissionCheckDecision `json:"decisions"`
}
