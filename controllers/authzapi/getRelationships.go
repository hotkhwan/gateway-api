// controllers/authzapi/getRelationships.go
package authzapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"klynx/config"
	"klynx/internal/services/authzsvc"
	"klynx/models/authzmod"
	"klynx/models/gmod"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// GetRelationshipsHandler godoc
// @Summary      Read relationships from Permify
// @Description  Read relationships filtered by subject or entity.
// @Tags         Authz
// @Accept       json
// @Produce      json
// @Param        request query authzmod.RelationshipQuery false "Query Parameters"

// @Failure      400 {object} gmod.ErrorMessageResponse "Bad Request – invalid query"
// @Failure      500 {object} gmod.ErrorMessageResponse "Internal Server Error – failed to call Permify"
// @Router       /authz/relationships [get]
// @Security     BearerAuth
func GetRelationshipsHandler(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/authzapi")
	ctx, span := tracer.Start(ctx, "Authz.GetRelationshipsHandler")
	defer span.End()

	// 1) เตรียม client สำหรับกรณี entity-based
	pc := authzsvc.NewPermifyClient()
	if pc == nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: "permify client not initialized",
			Status:  false,
		})
	}

	// 2) auto-bind query → RelationshipQuery
	var q authzmod.RelationshipQuery
	if err := c.QueryParser(&q); err != nil {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "invalid query parameters",
			Status:  false,
		})
	}

	if q.Page < 1 {
		q.Page = 1
	}
	if q.PerPages <= 0 {
		q.PerPages = 10
	}
	if strings.TrimSpace(q.SortField) == "" {
		q.SortField = "entity.id"
	}
	q.SortOrder = strings.ToLower(strings.TrimSpace(q.SortOrder))
	if q.SortOrder == "" {
		q.SortOrder = "asc"
	}

	subjectType := strings.TrimSpace(q.SubjectType)
	// ✅ ใช้ SubjectIds ตาม struct เดิม
	subjectID := strings.TrimSpace(q.SubjectIds)

	entityType := strings.TrimSpace(q.EntityType)
	entityIDsRaw := strings.TrimSpace(q.EntityIds)

	// === BRANCH A: entityType + entityIds → ใช้ client เดิม + filter subject ในแอป ===
	if entityType != "" && entityIDsRaw != "" {
		scope := []authzsvc.EntityRef{}
		ids := strings.Split(entityIDsRaw, ",")
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			scope = append(scope, authzsvc.EntityRef{
				Type: entityType,
				ID:   id,
			})
		}

		if len(scope) == 0 {
			return emptyRelationshipsResponse(c, q)
		}

		rels, err := pc.ReadExistingTuples(ctx, scope)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
				Code:    "ERROR",
				Message: err.Error(),
				Status:  false,
			})
		}

		// filter subject ถ้าระบุมา
		if subjectType != "" && subjectID != "" {
			filtered := make([]authzsvc.Relationship, 0, len(rels))
			for _, r := range rels {
				if r.Subject.Type == subjectType && r.Subject.ID == subjectID {
					filtered = append(filtered, r)
				}
			}
			rels = filtered
		}

		return paginateAndRespondRelationships(c, q, rels)
	}

	// === BRANCH B: ไม่มี entity แต่มี subjectType + subjectId → ยิง Permify REST โดยใช้ subject_filter ===
	if subjectType != "" && subjectID != "" {
		schemaVersion := config.CurrentSchemaVersion
		if strings.TrimSpace(schemaVersion) == "" {
			schemaVersion = "latest"
		}

		payload := map[string]interface{}{
			"metadata": map[string]interface{}{
				"schema_version": schemaVersion,
			},
			"filter": map[string]interface{}{
				"subject_filter": map[string]interface{}{
					"type": subjectType,
					"ids":  []string{subjectID},
				},
			},
		}

		data, _ := json.Marshal(payload)
		url := config.PermifyBaseURL + "/v1/tenants/" + config.PermifyTenantID + "/data/relationships/read"

		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		httpReq, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(data))
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
				Code:    "ERROR",
				Message: err.Error(),
				Status:  false,
			})
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
				Code:    "ERROR",
				Message: "permify read failed: " + string(body),
				Status:  false,
			})
		}

		var rr struct {
			Tuples []authzsvc.Relationship `json:"tuples"`
		}
		if err := json.Unmarshal(body, &rr); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
				Code:    "ERROR",
				Message: "failed to parse Permify response: " + err.Error(),
				Status:  false,
			})
		}

		rels := rr.Tuples
		return paginateAndRespondRelationships(c, q, rels)
	}

	// === BRANCH C: ไม่ระบุทั้ง entity และ subject → ว่าง ๆ ไปก่อน ===
	return emptyRelationshipsResponse(c, q)
}

// emptyRelationshipsResponse ส่ง response ว่าง ๆ พร้อม pagination
func emptyRelationshipsResponse(c *fiber.Ctx, q authzmod.RelationshipQuery) error {
	return c.JSON(fiber.Map{
		"details": []any{},
		"pagination": gmod.Pagination{
			Page:         q.Page,
			PerPages:     q.PerPages,
			TotalRecords: 0,
			TotalPages:   0,
			SortField:    q.SortField,
			SortOrder:    q.SortOrder,
		},
		"status": true,
	})
}

// paginateAndRespondRelationships ทำ sort + pagination แล้วตอบกลับ
func paginateAndRespondRelationships(
	c *fiber.Ctx,
	q authzmod.RelationshipQuery,
	rels []authzsvc.Relationship,
) error {
	// sort (ตาม entity.type / entity.id)
	sort.Slice(rels, func(i, j int) bool {
		ei, ej := rels[i].Entity, rels[j].Entity
		if q.SortOrder == "desc" {
			if ei.Type == ej.Type {
				return ei.ID > ej.ID
			}
			return ei.Type > ej.Type
		}
		if ei.Type == ej.Type {
			return ei.ID < ej.ID
		}
		return ei.Type < ej.Type
	})

	total := len(rels)
	start := (q.Page - 1) * q.PerPages
	if start > total {
		start = total
	}
	end := start + q.PerPages
	if end > total {
		end = total
	}
	pageList := rels[start:end]

	return c.JSON(fiber.Map{
		"details": pageList,
		"pagination": gmod.Pagination{
			Page:         q.Page,
			PerPages:     q.PerPages,
			TotalRecords: total,
			TotalPages:   (total + q.PerPages - 1) / q.PerPages,
			SortField:    q.SortField,
			SortOrder:    q.SortOrder,
		},
		"status": true,
	})
}
