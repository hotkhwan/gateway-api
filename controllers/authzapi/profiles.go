// controllers/authzapi/profiles.go
package authzapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/gmod"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel"
)

// ---- DI wiring ----
type listProfilesData struct {
	Details    []*authzmod.Profile `json:"details"`
	Pagination gmod.Pagination     `json:"pagination"`
}

var profileSvc *authzsvc.ProfileService

func SetProfileService(s *authzsvc.ProfileService) {
	profileSvc = s
}

// ---- Helpers ----

func normalizeCRUD(vv []string) []string {
	out := make([]string, 0, len(vv))
	for _, v := range vv {
		v = strings.ToLower(strings.TrimSpace(v))
		switch v {
		case "c", "create":
			out = append(out, "create")
		case "r", "read", "view":
			out = append(out, "read")
		case "u", "update", "edit":
			out = append(out, "update")
		case "d", "delete", "remove":
			out = append(out, "delete")
		}
	}
	return out
}

func toProfileItems(pis []authzmod.PublishItem) []authzmod.ProfileItem {
	var items []authzmod.ProfileItem
	for _, it := range pis {
		res := strings.TrimSpace(it.ResourceID)
		if res == "" {
			continue
		}
		subject := strings.TrimSpace(it.Relationship)
		// รองรับ legacy subjectType/subjectId
		if subject == "" && it.SubjectType != "" && it.SubjectID != "" {
			subject = strings.TrimSpace(it.SubjectType) + ":" + strings.TrimSpace(it.SubjectID)
		}
		if it.Public {
			items = append(items, authzmod.ProfileItem{
				ID:       res + ":public",
				Action:   "public",
				Resource: res,
				Subject:  "group:g_public",
			})
		}
		for _, crud := range normalizeCRUD(it.CRUD) {
			action := crud
			switch crud {
			case "read":
				action = "read"
			case "update":
				action = "update"
			case "delete":
				action = "delete"
			case "create":
				// ถ้าอยาก map create → editor path ก็แปลงเป็น update
				action = "update"
			}
			items = append(items, authzmod.ProfileItem{
				ID:       res + ":" + subject + ":" + action,
				Action:   action,
				Resource: res,
				Subject:  subject, // e.g. "role:PF_OFFICE" / "group:g_inno"
			})
		}
	}
	return items
}

// ---- Handlers ----

// POST /authz/profiles
func CreateProfileHandler(c *fiber.Ctx) error {
	if profileSvc == nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: "profile service not initialized",
			Status:  false,
		})
	}

	var in authzmod.Profile
	if err := c.BodyParser(&in); err != nil {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "invalid json body",
			Status:  false,
		})
	}
	now := time.Now().UTC()
	in.CreatedAt = now
	in.UpdatedAt = now

	if err := profileSvc.CreateProfile(c.UserContext(), &in); err != nil {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: err.Error(),
			Status:  false,
		})
	}
	resp := gmod.SuccessDataResponse{
		Code:    "SUCCESS",
		Message: "profile created successfully",
		Status:  true,
		Data:    in, // หรือ nil ใน prod
	}
	// 201 แบบ payload เดิมของคุณ
	return c.Status(http.StatusCreated).JSON(resp)
}

// GET /authz/profiles
func ListProfilesHandler(c *fiber.Ctx) error {
	if profileSvc == nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: "profile service not initialized",
			Status:  false,
		})
	}

	// simple paging
	page := c.QueryInt("page", 1)
	perPage := c.QueryInt("perPages", 10)
	sortField := c.Query("sortField", "createdAt")
	sortOrder := c.Query("sortOrder", "asc")

	filter := make(map[string]any)
	opts := fiberToMongoFindOptions(page, perPage, sortField, sortOrder)

	ctx := c.UserContext()
	list, err := profileSvc.ListProfiles(ctx, filter, opts)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: err.Error(),
			Status:  false,
		})
	}
	total, _ := profileSvc.CountProfiles(ctx, filter)
	totalPages := (int(total) + perPage - 1) / perPage

	data := listProfilesData{
		Details: list,
		Pagination: gmod.Pagination{
			Page:         page,
			PerPages:     perPage,
			TotalRecords: int(total),
			TotalPages:   totalPages,
			SortField:    sortField,
			SortOrder:    sortOrder,
		},
	}

	// DEV: ใส่ Data = data
	// PROD: ถ้าอยากให้เหลือแค่ code/message/status ก็ set Data=nil
	resp := gmod.SuccessDataResponse{
		Code:    "SUCCESS",
		Message: "profiles fetched",
		Status:  true,
		Data:    data,
	}

	return c.JSON(resp)

	// return c.JSON(fiber.Map{
	// 	"details": list,
	// 	"pagination": gmod.Pagination{
	// 		Page:         page,
	// 		PerPages:     perPage,
	// 		TotalRecords: int(total),
	// 		TotalPages:   totalPages,
	// 		SortField:    sortField,
	// 		SortOrder:    sortOrder,
	// 	},
	// 	"status": true,
	// })
}

// PATCH /authz/profiles/:code
func UpdateProfileHandler(c *fiber.Ctx) error {
	if profileSvc == nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: "profile service not initialized",
			Status:  false,
		})
	}
	code := c.Params("code")
	if code == "" {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "missing profile code",
			Status:  false,
		})
	}

	var in map[string]any
	if err := c.BodyParser(&in); err != nil {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "invalid json body",
			Status:  false,
		})
	}
	// อัปเดต updatedAt เป็น camelCase ให้ตรงกับ bson:"updatedAt"
	in["updatedAt"] = time.Now().UTC()

	if err := profileSvc.UpdateProfile(c.UserContext(), code, in); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: err.Error(),
			Status:  false,
		})
	}
	return c.JSON(gmod.SuccessMessageResponse{
		Code:    "SUCCESS",
		Message: "profile updated",
		Status:  true,
	})
}

// PublishProfileHandler godoc
// @Summary      Publish profile as new version
// @Description  Replace profile items and create new version for given code
// @Tags         Authz
// @Accept       json
// @Produce      json
// @Param        code   path      string                       true  "Profile code"
// @Param        body   body      authzmod.PublishProfileRequest true  "Publish profile payload"
// @Success      200    {object}  map[string]interface{}
// @Failure      400    {object}  gmod.ErrorMessageResponse
// @Failure      500    {object}  gmod.ErrorMessageResponse
// @Router       /authz/profiles/{code}/publish [post]
// @Security     BearerAuth
func PublishProfileHandler(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/authzapi")
	ctx, span := tracer.Start(ctx, "Authz.PublishProfileHandler")
	defer span.End()

	if profileSvc == nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: "profile service not initialized",
			Status:  false,
		})
	}
	code := c.Params("code")
	if code == "" {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "missing profile code",
			Status:  false,
		})
	}

	var in authzmod.PublishProfileRequest
	if err := c.BodyParser(&in); err != nil {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "invalid json body",
			Status:  false,
		})
	}

	// 1) replace items
	items := toProfileItems(in.Items)
	if err := profileSvc.UpdateProfile(ctx, code, map[string]any{
		"items":     items,
		"updatedAt": time.Now().UTC(),
	}); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: err.Error(),
			Status:  false,
		})
	}

	// 2) new version
	v, err := profileSvc.PublishVersion(ctx, code)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: err.Error(),
			Status:  false,
		})
	}

	// 3) note
	if in.Note != "" {
		_ = profileSvc.AddVersionNote(ctx, code, v.Version, in.Note)
	}

	// ตอบ body ให้เหมือนที่คุณเทสอยู่
	// return c.JSON(fiber.Map{
	// 	"profileCode": v.ProfileCode,
	// 	"version":     v.Version,
	// 	"items":       v.Items,
	// 	"note":        in.Note,
	// 	"createdAt":   v.CreatedAt.UTC().Format(time.RFC3339Nano),
	// })
	data := fiber.Map{
		"profileCode": v.ProfileCode,
		"version":     v.Version,
		"items":       v.Items,
		"note":        in.Note,
		"createdAt":   v.CreatedAt.UTC().Format(time.RFC3339Nano),
	}

	resp := gmod.SuccessDataResponse{
		Code:    "SUCCESS",
		Message: "profile published successfully",
		Status:  true,
		Data:    data, // หรือ nil ใน prod
	}

	return c.JSON(resp)
}

// POST /authz/profiles/:code/plan
func PlanProfileHandler(c *fiber.Ctx) error {
	if profileSvc == nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: "profile service not initialized",
			Status:  false,
		})
	}
	code := c.Params("code")
	if code == "" {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "missing profile code",
			Status:  false,
		})
	}
	var in authzmod.PlanProfileRequest
	_ = c.BodyParser(&in)

	// plan, err := profileSvc.PlanChanges(c.UserContext(), code, in.Version)
	// if err != nil {
	// 	return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
	// 		Code:    "ERROR",
	// 		Message: err.Error(),
	// 		Status:  false,
	// 	})
	// }
	// return c.JSON(plan)
	plan, err := profileSvc.PlanChanges(c.UserContext(), code, in.Version)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: err.Error(),
			Status:  false,
		})
	}

	resp := gmod.SuccessDataResponse{
		Code:    "SUCCESS",
		Message: "profile plan generated",
		Status:  true,
		Data:    plan, // หรือ nil ใน prod
	}

	return c.JSON(resp)

}

// POST /authz/profiles/:code/apply
// ใช้ Header: Idempotency-Key (ถ้าให้แนบ)
func ApplyProfileHandler(c *fiber.Ctx) error {
	if profileSvc == nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: "profile service not initialized",
			Status:  false,
		})
	}
	code := c.Params("code")
	if code == "" {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "missing profile code",
			Status:  false,
		})
	}
	var in authzmod.ApplyProfileRequest
	_ = c.BodyParser(&in)
	idemKey := strings.TrimSpace(c.Get("Idempotency-Key"))

	plan, err := profileSvc.PlanChanges(c.UserContext(), code, 0)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: err.Error(),
			Status:  false,
		})
	}

	if err := profileSvc.ApplyChanges(c.UserContext(), plan, idemKey); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.JSON(gmod.SuccessMessageResponse{
		Code:    "SUCCESS",
		Message: "profile apply successfully",
		Status:  true,
	})
}

// GET /authz/profiles/:code/drift
func DriftProfileHandler(c *fiber.Ctx) error {
	return c.Status(http.StatusNotImplemented).JSON(gmod.ErrorMessageResponse{
		Code:    "NOT_IMPLEMENTED",
		Message: "drift endpoint not implemented yet",
		Status:  false,
	})
}

// POST /authz/profiles/:code/reconcile
func ReconcileProfileHandler(c *fiber.Ctx) error {
	return c.Status(http.StatusNotImplemented).JSON(gmod.ErrorMessageResponse{
		Code:    "NOT_IMPLEMENTED",
		Message: "reconcile endpoint not implemented yet",
		Status:  false,
	})
}

// ---- utils ----

func fiberToMongoFindOptions(page, perPage int, sortField, sortOrder string) *options.FindOptions {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 10
	}
	skip := int64((page - 1) * perPage)
	limit := int64(perPage)
	opt := options.Find().SetSkip(skip).SetLimit(limit)
	order := 1
	if strings.ToLower(sortOrder) == "desc" {
		order = -1
	}
	if sortField == "" {
		sortField = "createdAt"
	}
	// ตอนนี้ field ใน Mongo เป็น camelCase อยู่แล้ว
	opt.SetSort(map[string]int{sortField: order})
	return opt
}

// DELETE /authz/profiles/:code
func DeleteProfileHandler(c *fiber.Ctx) error {
	if profileSvc == nil {
		return c.Status(http.StatusInternalServerError).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: "profile service not initialized",
			Status:  false,
		})
	}

	id := c.Params("code")
	if id == "" {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "INVALID_REQUEST",
			Message: "missing profile id",
			Status:  false,
		})
	}

	// เรียก service เพื่อลบ
	if err := profileSvc.DeleteProfile(c.UserContext(), id); err != nil {
		return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
			Code:    "ERROR",
			Message: err.Error(),
			Status:  false,
		})
	}

	// ตอบกลับเมื่อสำเร็จ
	resp := gmod.SuccessMessageResponse{
		Code:    "SUCCESS",
		Message: "profile deleted successfully",
		Status:  true,
	}
	// 200 OK
	return c.Status(http.StatusOK).JSON(resp)
}
