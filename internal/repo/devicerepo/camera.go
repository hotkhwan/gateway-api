// internal/repo/devicerepo/cameraRepo.go
package devicerepo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/devmod"
	"github.com/hotkhwan/gateway-api/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collCamera = "camera"

type CameraRepo struct {
	collection string
}

func NewCameraRepo() *CameraRepo {
	return &CameraRepo{collection: collCamera}
}

func (r *CameraRepo) EnsureIndexes(ctx context.Context) error {
	return stomongo.CreateIndexes(ctx, r.collection, []mongo.IndexModel{
		// camId เป็น public ID → unique
		{
			Keys:    bson.D{{Key: "camId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "orgId", Value: 1}}},
		{Keys: bson.D{{Key: "orgId", Value: 1}}},
		{Keys: bson.D{{Key: "ip", Value: 1}}},
	})
}

// ============================================================
// Insert — generate camId (UUID) เพื่อใช้เป็น public ID + Permify
// ============================================================

func (r *CameraRepo) Insert(ctx context.Context, input devmod.CreateCameraInput) (string, error) {
	if input.MapVisibility == "" {
		input.MapVisibility = "inherit"
	}

	camId := uuid.New().String() // ← public ID ใช้ใน Permify tuple + API route

	doc := bson.M{
		"camId":         camId, // ← index unique
		"tenantId":      input.TenantID,
		"orgId":         input.OrgID,
		"name":          input.Name,
		"user":          input.User,
		"password":      input.Password,
		"url":           input.URL,
		"ip":            input.IP,
		"district":      input.District,
		"lat":           input.Lat,
		"lng":           input.Lng,
		"brand":         input.Brand,
		"status":        true,
		"state":         "active",
		"mapVisibility": input.MapVisibility,
		"createdBy":     input.CallerID,
		"createAt":      time.Now(),
	}
	if len(input.Roi) > 0 {
		doc["roi"] = input.Roi
	}

	_, err := stomongo.InsertOne(ctx, r.collection, doc)
	if err != nil {
		return "", err
	}
	return camId, nil // ← return UUID ไม่ใช่ ObjectID
}

// ============================================================
// BulkInsert
// ============================================================

func (r *CameraRepo) BulkInsert(ctx context.Context, tenantId, orgId, callerID string, items []devmod.BulkImportItem) ([]string, error) {
	docs := make([]interface{}, 0, len(items))
	camIds := make([]string, 0, len(items))

	for _, item := range items {
		camId := uuid.New().String()
		camIds = append(camIds, camId)
		docs = append(docs, bson.M{
			"camId":         camId,
			"tenantId":      tenantId,
			"orgId":         orgId,
			"name":          item.Name,
			"url":           item.URL,
			"ip":            item.IP,
			"lat":           item.Lat,
			"lng":           item.Lng,
			"district":      item.District,
			"brand":         item.Brand,
			"status":        true,
			"state":         "active",
			"mapVisibility": "inherit",
			"createdBy":     callerID,
			"createAt":      time.Now(),
		})
	}

	// stomongo.InsertMany returns []primitive.ObjectID — เราไม่ต้องการ ObjectID
	// camIds สร้างไว้แล้ว ก่อน insert → consistent
	_, err := stomongo.InsertMany(ctx, r.collection, docs, nil)
	if err != nil {
		return nil, err
	}
	return camIds, nil
}

// ============================================================
// FindByCamID — ค้นด้วย public UUID
// ============================================================

func (r *CameraRepo) FindByCamID(ctx context.Context, camId string) (*devmod.CameraMongo, error) {
	var result devmod.CameraMongo
	err := stomongo.FindOne(ctx, r.collection, bson.M{"camId": camId}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (r *CameraRepo) FindByCamIDAndOrg(ctx context.Context, camId, orgId string) (*devmod.CameraMongo, error) {
	cam, err := r.FindByCamID(ctx, camId)
	if err != nil {
		return nil, err
	}
	if cam.OrgID != orgId {
		return nil, ErrCrossOrgAccess
	}
	return cam, nil
}

func (r *CameraRepo) GetOrgID(ctx context.Context, camId string) (string, error) {
	cam, err := r.FindByCamID(ctx, camId)
	if err != nil {
		return "", err
	}
	return cam.OrgID, nil
}

// ============================================================
// Update — PATCH (ส่ง fields ตรง ไม่ห่อ $set เอง)
// ============================================================

func (r *CameraRepo) Update(ctx context.Context, camId string, input devmod.UpdateCameraInput) error {
	setFields := bson.M{
		"name":     input.Name,
		"user":     input.User,
		"password": input.Password,
		"url":      input.URL,
		"district": input.District,
		"lat":      input.Lat,
		"lng":      input.Lng,
	}
	if len(input.Roi) > 0 {
		setFields["roi"] = input.Roi
	}
	if input.MapVisibility != "" {
		setFields["mapVisibility"] = input.MapVisibility
	}
	// stomongo.UpdateOne ห่อ $set ให้ → ส่ง fields โดยตรง
	res, err := stomongo.UpdateOne(ctx, r.collection, bson.M{"camId": camId}, setFields)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================
// HardDelete — ลบ doc ออกจาก Mongo ถาวร
// เรียกหลังจาก Permify tuple ถูกลบแล้ว
// ถ้า Mongo fail → orphan doc แต่ authz ปิดแล้ว (tuple ลบไปก่อน)
// ============================================================

func (r *CameraRepo) HardDelete(ctx context.Context, camId string) error {
	res, err := stomongo.DeleteOne(ctx, r.collection, bson.M{"camId": camId})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================
// List
// ============================================================

type CameraListOptions struct {
	Search    string
	GroupID   string
	Page      int
	PerPage   int
	SortField string
	SortOrder string
}

func (r *CameraRepo) List(ctx context.Context, tenantId, orgId string, opts CameraListOptions) ([]devmod.CameraMongo, int64, error) {
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.PerPage <= 0 {
		opts.PerPage = 10
	}
	if opts.SortField == "" {
		opts.SortField = "createAt"
	}
	sortVal := -1
	if opts.SortOrder == "asc" {
		sortVal = 1
	}

	filter := bson.M{"tenantId": tenantId, "orgId": orgId}
	if opts.Search != "" {
		filter["name"] = bson.M{"$regex": opts.Search, "$options": "i"}
	}

	var results []devmod.CameraMongo
	if err := stomongo.FindPaginated(ctx, r.collection, filter, stomongo.PaginateOptions{
		Page:    opts.Page,
		PerPage: opts.PerPage,
		Sort:    bson.D{{Key: opts.SortField, Value: sortVal}},
	}, &results); err != nil {
		return nil, 0, err
	}
	total, err := stomongo.Count(ctx, r.collection, filter)
	if err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// ============================================================
// ExistsByNameInOrg — ตรวจสอบชื่อกล้องซ้ำใน org เดียวกัน
// excludeCamId ใช้ตอน Update เพื่อ exclude ตัวเอง
// ============================================================

func (r *CameraRepo) ExistsByNameInOrg(ctx context.Context, orgId, name, excludeCamId string) (bool, error) {
	filter := bson.M{"orgId": orgId, "name": name}
	if excludeCamId != "" {
		filter["camId"] = bson.M{"$ne": excludeCamId}
	}
	count, err := stomongo.Count(ctx, r.collection, filter)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ============================================================
// FindByCamIDs — fetch multiple cameras by camId list + pagination
// ============================================================

func (r *CameraRepo) FindByCamIDs(ctx context.Context, camIds []string, search, sortField, sortOrder string, page, perPage int) ([]devmod.CameraMongo, int64, error) {
	if len(camIds) == 0 {
		return nil, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 10
	}
	if sortField == "" {
		sortField = "createAt"
	}
	sortVal := -1
	if sortOrder == "asc" {
		sortVal = 1
	}

	filter := bson.M{"camId": bson.M{"$in": camIds}}
	if search != "" {
		filter["name"] = bson.M{"$regex": search, "$options": "i"}
	}

	var results []devmod.CameraMongo
	if err := stomongo.FindPaginated(ctx, r.collection, filter, stomongo.PaginateOptions{
		Page:    page,
		PerPage: perPage,
		Sort:    bson.D{{Key: sortField, Value: sortVal}},
	}, &results); err != nil {
		return nil, 0, err
	}
	total, err := stomongo.Count(ctx, r.collection, filter)
	if err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// ============================================================
// FindDuplicateIPs
// ============================================================

func (r *CameraRepo) FindDuplicateIPs(ctx context.Context, orgId string, ips []string) ([]string, error) {
	if len(ips) == 0 {
		return nil, nil
	}
	var docs []devmod.CameraMongo
	err := stomongo.Find(ctx, r.collection, bson.M{
		"orgId": orgId,
		"ip":    bson.M{"$in": ips},
	}, options.Find().SetProjection(bson.M{"ip": 1}), &docs)
	if err != nil {
		return nil, err
	}
	found := make([]string, 0, len(docs))
	for _, d := range docs {
		found = append(found, d.IP)
	}
	return found, nil
}

// ============================================================
// ToDTO — ใช้ CamID เป็น public ID
// ============================================================

func ToDTO(d *devmod.CameraMongo) devmod.CameraDTO {
	return devmod.CameraDTO{
		ID:            d.CamId, // ← UUID ไม่ใช่ _id.Hex()
		TenantID:      d.TenantID,
		OrgID:         d.OrgID,
		Name:          d.Name,
		User:          d.User,
		URL:           d.URL,
		IP:            d.IP,
		District:      d.District,
		Lat:           d.Lat,
		Lng:           d.Lng,
		Brand:         d.Brand,
		Status:        d.Status,
		Roi:           d.Roi,
		MapVisibility: d.MapVisibility,
		CreateAt:      utils.FormatTimeOrEmpty(d.CreateAt),
		UpdateAt:      utils.FormatTimeOrEmpty(d.UpdateAt),
	}
}

// ============================================================
// FindByID (internal ObjectID) — เผื่อ migration หรือ admin tool
// ============================================================

func (r *CameraRepo) FindByObjectID(ctx context.Context, id primitive.ObjectID) (*devmod.CameraMongo, error) {
	var result devmod.CameraMongo
	err := stomongo.FindOne(ctx, r.collection, bson.M{"_id": id}, &result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (r *CameraRepo) GetDeviceType(_ context.Context, _ string) (string, error) {
	return "camera", nil
}

// Delete — alias ของ HardDelete เพื่อ satisfy devicesvc.CameraRepo interface
func (r *CameraRepo) Delete(ctx context.Context, camId string) error {
	return r.HardDelete(ctx, camId)
}
