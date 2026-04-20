// models/devmodel/device.go
package devmod

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Device struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	User        string                `json:"user"`
	Password    string                `json:"password"`
	URL         string                `json:"url"`
	District    string                `json:"district"`
	Lat         float64               `json:"lat"`
	Lng         float64               `json:"lng"`
	AtaWsFlvUrl string                `json:"ataWsFlvUrl"`
	Brand       string                `json:"brand"`
	Status      bool                  `json:"status"`
	State       string                `json:"state"`
	Roi         [][]map[string]string `json:"roi,omitempty"`
	Public      bool                  `json:"public,omitempty"`
	CreateAt    string                `json:"createAt"`
	UpdateAt    string                `json:"updateAt"`
	Access      Access                `json:"access,omitempty"`
	GroupIds    []string              `json:"groupIds,omitempty"`
}

type DeviceMongo struct {
	ID          primitive.ObjectID    `bson:"_id,omitempty"`
	Name        string                `bson:"name"`
	User        string                `bson:"user"`
	Password    string                `bson:"password"`
	URL         string                `bson:"url"`
	District    string                `bson:"district"`
	Lat         float64               `bson:"lat"`
	Lng         float64               `bson:"lng"`
	AtaWsFlvUrl string                `bson:"ataWsFlvUrl"`
	Brand       string                `bson:"brand"`
	Status      bool                  `bson:"status"`
	State       string                `bson:"state"`
	Roi         [][]map[string]string `bson:"roi,omitempty"`
	Public      bool                  `bson:"public,omitempty"`
	CreateAt    time.Time             `bson:"createAt,omitempty"`
	UpdateAt    time.Time             `bson:"updateAt,omitempty"`
	Access      Access                `bson:"access,omitempty"`
	GroupIds    []string              `bson:"groupIds,omitempty"`
}

type CamAIMessage struct {
	DeviceID  string    `json:"deviceId"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	ImgPath1  string    `json:"imgPath1"`
	ImgPath2  string    `json:"imgPath2"`
	// optional: Camera location, confidence, etc.
}

// DeviceRequest represents the expected payload for creating a device.
// swagger:model
type DeviceRequest struct {
	// required: true
	Name string `json:"name" example:"Camera-01"`
	// required: true
	User string `json:"user" example:"admin"`
	// required: true
	Password string `json:"password" example:"secret123"`
	// required: true
	URL string `json:"url" example:"rtsp://192.168.1.100/stream"`
	// required: true
	District string   `json:"district" example:"Prawet"`
	Lat      *float64 `json:"lat" example:"13.6178196"`
	Lng      *float64 `json:"lng" example:"100.6956348"`
	// required: true
	Brand string `json:"brand" example:"Dahua"`
}

// ---- Swagger helper type (เพื่อเอกสาร) ----
type DeviceDetailSwagger struct {
	Code    string `json:"code" example:"SUCCESS"`
	Message string `json:"message" example:"Device retrieved successfully"`
	Status  bool   `json:"status" example:"true"`
	Details Device `json:"details"`
}

type DeviceListResponse struct {
	Details    []Device   `json:"details"`
	Pagination Pagination `json:"pagination"`
	Online     int        `json:"online"`
	Offline    int        `json:"offline"`
	Status     bool       `json:"status"`
}

type Pagination struct {
	Page         int    `json:"page"`
	PerPage      int    `json:"perPage"`
	TotalRecords int    `json:"totalRecords"`
	TotalPages   int    `json:"totalPages"`
	SortField    string `json:"sortField"`
	SortOrder    string `json:"sortOrder"`
}

// Access
type Access struct {
	Allow  Allow  `json:"allow"`
	Source Source `json:"source"`
}

type Allow struct {
	Read   bool `json:"read"`
	Create bool `json:"create"`
	Update bool `json:"update"`
	Delete bool `json:"delete"`
}

type Source struct {
	Mode            string   `json:"mode"`
	MatchedGroupIDs []string `json:"matchedGroupIds"`
	Subject         Subject  `json:"subject"`
}

type Subject struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// PermissionSubjectCheckRequest
type PermissionSubjectCheckRequest struct {
	Metadata struct {
		SchemaVersion string `json:"schema_version,omitempty"`
		SnapToken     string `json:"snap_token,omitempty"`
		Depth         int    `json:"depth" validate:"required"`
	} `json:"metadata" validate:"required"`
	Entity struct {
		Type string `json:"type" validate:"required"` // เช่น "resource"
		ID   string `json:"id" validate:"required"`
	} `json:"entity" validate:"required"`
	Permission []string `json:"permission" validate:"required"`
	Subject    struct {
		Type string `json:"type" validate:"required"` // เช่น "user", "group", "role"
		ID   string `json:"id" validate:"required"`
	} `json:"subject" validate:"required"`
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
