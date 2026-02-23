// models/devmod/camera.go
package devmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ============================================================
// CameraMongo — Mongo document (internal)
// _id = ObjectId (internal only, never expose to API/Permify)
// Permify tuple ใช้ _id.Hex() ได้เพราะ device เป็น internal entity
// ============================================================

type CameraMongo struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	CamId       string             `bson:"camId"`
	TenantID    string             `bson:"tenantId"`
	OrgID       string             `bson:"orgId"`
	Name        string             `bson:"name"`
	User        string             `bson:"user,omitempty"`
	Password    string             `bson:"password,omitempty"`
	URL         string             `bson:"url,omitempty"`
	IP          string             `bson:"ip,omitempty"`
	District    string             `bson:"district,omitempty"`
	Lat         float64            `bson:"lat,omitempty"`
	Lng         float64            `bson:"lng,omitempty"`
	AtaWsFlvUrl string             `bson:"ataWsFlvUrl,omitempty"`
	Brand       string             `bson:"brand,omitempty"`
	Status      bool               `bson:"status"`
	State       string             `bson:"state,omitempty"` // create | update | delete
	Roi         []interface{}      `bson:"roi,omitempty"`
	GroupIDs    []string           `bson:"groupIds"`

	// Map visibility — tri-state override
	// "inherit" = ใช้ค่าจาก group.mapVisibility
	// "forcePublic" = โชว์บน map เสมอ
	// "forcePrivate" = ไม่โชว์บน map เสมอ
	MapVisibilityOverride string `bson:"mapVisibilityOverride"` // inherit | forcePublic | forcePrivate

	// Legacy field (backward compat)
	Public bool `bson:"public,omitempty"`

	SyncStatus string    `bson:"syncStatus,omitempty"` // pending | synced | failed
	IsDeleted  bool      `bson:"isDeleted,omitempty"`
	CreatedBy  string    `bson:"createdBy,omitempty"`
	CreateAt   time.Time `bson:"createAt,omitempty"`
	UpdateAt   time.Time `bson:"updateAt,omitempty"`
}

// ============================================================
// CameraDTO — API response (เอา _id ออก, map เป็น string id)
// ============================================================

type CameraDTO struct {
	ID          string        `json:"id"` // _id.Hex()
	TenantID    string        `json:"tenantId"`
	OrgID       string        `json:"orgId"`
	Name        string        `json:"name"`
	User        string        `json:"user,omitempty"`
	URL         string        `json:"url,omitempty"`
	IP          string        `json:"ip,omitempty"`
	District    string        `json:"district,omitempty"`
	Lat         float64       `json:"lat,omitempty"`
	Lng         float64       `json:"lng,omitempty"`
	AtaWsFlvUrl string        `json:"ataWsFlvUrl,omitempty"`
	Brand       string        `json:"brand,omitempty"`
	Status      bool          `json:"status"`
	State       string        `json:"state,omitempty"`
	Roi         []interface{} `json:"roi,omitempty"`
	GroupIDs    []string      `json:"groupIds"`

	MapVisibilityOverride string `json:"mapVisibilityOverride"`

	SyncStatus string `json:"syncStatus,omitempty"`
	CreateAt   string `json:"createAt,omitempty"`
	UpdateAt   string `json:"updateAt,omitempty"`
}

// ============================================================
// CreateCameraInput — service input
// ============================================================

type CreateCameraInput struct {
	CamID                 string
	TenantID              string
	OrgID                 string
	CallerID              string
	Name                  string
	User                  string
	Password              string
	URL                   string
	IP                    string
	District              string
	Lat                   float64
	Lng                   float64
	AtaWsFlvUrl           string
	Brand                 string
	Roi                   []interface{}
	MapVisibilityOverride string // inherit | forcePublic | forcePrivate
}

// ============================================================
// UpdateCameraInput — service input
// ============================================================

type UpdateCameraInput struct {
	Name                  string
	User                  string
	Password              string
	URL                   string
	District              string
	Lat                   float64
	Lng                   float64
	Roi                   []interface{}
	MapVisibilityOverride string
}

// ============================================================
// BulkImportItem — row จาก CSV/XLSX หลัง parse
// ============================================================

type BulkImportItem struct {
	Name     string
	URL      string
	IP       string
	Lat      float64
	Lng      float64
	District string
	Brand    string
	Groups   []string               // optional group names
	Raw      map[string]interface{} // raw row สำหรับ debug
}

// ============================================================
// BulkImportResult — result ต่อ row
// ============================================================

type BulkImportResult struct {
	Row     int    `json:"row"`
	Name    string `json:"name,omitempty"`
	ID      string `json:"id,omitempty"` // inserted _id.Hex()
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}
