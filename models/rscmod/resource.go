// models/rscmod/resource.go
package rscmod

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Resource struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	CanonicalId string             `bson:"canonicalId" json:"canonicalId"`
	Provider    string             `bson:"provider" json:"provider"`
	Type        string             `bson:"type" json:"type"`
	Id          string             `bson:"id,omitempty" json:"idAlias,omitempty"`
	ExternalId  string             `bson:"externalId,omitempty" json:"externalId,omitempty"`
	DisplayName string             `bson:"displayName" json:"displayName"`
	Icon        string             `bson:"icon,omitempty" json:"icon,omitempty"`
	Path        string             `bson:"path,omitempty" json:"path,omitempty"`
	Tags        []string           `bson:"tags,omitempty" json:"tags,omitempty"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// ใช้ใน POST/PUT
type ResourceUpsert struct {
	CanonicalId string   `json:"canonicalId"`
	Provider    string   `json:"provider"`
	Type        string   `json:"type"`
	Id          string   `json:"id,omitempty"`
	ExternalId  string   `json:"externalId,omitempty"`
	DisplayName string   `json:"displayName"`
	Icon        string   `json:"icon,omitempty"`
	Path        string   `json:"path,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// -------- Pagination DTOs --------

type Pagination struct {
	Page         int    `json:"page"`
	PerPages     int    `json:"perPages"`
	TotalRecords int    `json:"totalRecords"`
	TotalPages   int    `json:"totalPages"`
	SortField    string `json:"sortField"`
	SortOrder    string `json:"sortOrder"`
}

type ResourcePaginationResponse struct {
	Details    []Resource `json:"details"`
	Pagination Pagination `json:"pagination"`
	Status     bool       `json:"status"`
}

// ส่งรูปแบบ pagination เดิมให้ controller เรียก
func SendPagination(c fiber.Ctx, items []Resource, pag Pagination) error {
	resp := ResourcePaginationResponse{
		Details:    items,
		Pagination: pag,
		Status:     true,
	}
	return c.JSON(resp)
}
