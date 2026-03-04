// models/systemmod/edgemod.go
package systemmod

import (
	"time"

	"github.com/hotkhwan/gateway-api/internal/crypto/secretbox"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type EdgeType string

const (
	EdgeATA  EdgeType = "ata"
	EdgeSVMS EdgeType = "svms"
	EdgeIBOC EdgeType = "iboc"
)

// ===== Requests =====

type EdgeCreateReq struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	URL       string `json:"url"`
	TLS       bool   `json:"tls"`
	Name      string `json:"name"`
	APIKey    any    `json:"apiKey,omitempty"`    // ATA only
	APISecret string `json:"apiSecret,omitempty"` // ATA only
}

type EdgeUpdateReq struct {
	Username *string `json:"username,omitempty"`
	Password *string `json:"password,omitempty"`
	Name     *string `json:"name,omitempty"`
	URL      *string `json:"url,omitempty"`
	TLS      *bool   `json:"tls,omitempty"`

	APIKey    *any    `json:"apiKey,omitempty"`
	APISecret *string `json:"apiSecret,omitempty"`
}

// ===== Mongo document (collection: system_edges) =====

type EdgeDoc struct {
	ID       any      `bson:"_id,omitempty" json:"id"`
	Type     EdgeType `bson:"type" json:"type"`
	Username string   `bson:"username" json:"username"`
	Name     string   `bson:"name" json:"name"`
	URL      string   `bson:"url" json:"url"`
	TLS      bool     `bson:"tls" json:"tls"`

	PassEnc      *secretbox.EncBlob `bson:"passEnc,omitempty" json:"-"`
	APIKey       any                `bson:"apiKey,omitempty" json:"apiKey,omitempty"`
	APISecretEnc *secretbox.EncBlob `bson:"apiSecretEnc,omitempty" json:"-"`

	// soft delete
	IsDeleted bool      `bson:"isDeleted,omitempty" json:"isDeleted,omitempty"`
	State     string    `bson:"state,omitempty" json:"state,omitempty"`
	DeletedAt time.Time `bson:"deletedAt,omitempty" json:"deletedAt,omitempty"`
	DeletedBy string    `bson:"deletedBy,omitempty" json:"deletedBy,omitempty"`

	CreatedAt time.Time `bson:"createdAt,omitempty" json:"createdAt,omitempty"`
	UpdatedAt time.Time `bson:"updatedAt,omitempty" json:"updatedAt,omitempty"`
}

// ===== List DTO =====

type EdgeListItem struct {
	ID        string    `json:"id"`
	Type      EdgeType  `json:"type"`
	Name      string    `json:"name"`
	Username  string    `json:"username"`
	URL       string    `json:"url"`
	TLS       bool      `json:"tls"`
	APIKey    any       `json:"apiKey,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// ===== Create response (id at root) =====

type EdgeCreateSuccessResponse struct {
	Code    string `json:"code" example:"SUCCESS"`
	ID      string `json:"id" example:"694a5ebe44cdee341c2f687a"`
	Message string `json:"message" example:"edge created"`
	Status  bool   `json:"status" example:"true"`
}

// ===== Update/Delete responses =====

type EdgeUpdateSuccessResponse struct {
	Code    string `json:"code" example:"SUCCESS"`
	Message string `json:"message" example:"edge updated"`
	Status  bool   `json:"status" example:"true"`
}

type EdgeDeleteSuccessResponse struct {
	Code    string `json:"code" example:"SUCCESS"`
	Message string `json:"message" example:"edge deleted"`
	Status  bool   `json:"status" example:"true"`
}

// ===== Internal config struct (not exposed) =====
type EdgeConfig struct {
	ID           primitive.ObjectID `bson:"_id"`
	Type         string             `bson:"type"`
	Username     string             `bson:"username"`
	URL          string             `bson:"url"`
	TLS          bool               `bson:"tls"`
	PassEnc      bson.M             `bson:"passEnc"`
	APIKey       int64              `bson:"apiKey,omitempty"`
	APISecretEnc bson.M             `bson:"apiSecretEnc,omitempty"`
}

type EdgeSSOURLDetail struct {
	SSOUrl string `json:"ssoUrl"`
}

type EdgeSSOURLSuccessResponse struct {
	Code   string           `json:"code"`
	Detail EdgeSSOURLDetail `json:"detail"`
	Status bool             `json:"status"`
}
