// internal/adapters/repo/stomongo/softDelete.go
package stomongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type SoftDeleteOptions struct {
	// ค่าเริ่มต้น: state="archived"
	State string
	// ล้าง key รูปแบบเดิม ๆ
	ClearPhotos bool
	// รายชื่อ external namespaces ที่ต้อง set state=deleted และ syncedAt=now
	ExternalNamespaces []string
	// ฟิลด์เสริมอื่น ๆ (เช่น deletedReason, runId)
	ExtraSet   bson.M
	ExtraUnset bson.M
}

// สร้าง $set/$unset ตาม SoftDeleteOptions
func buildSoftDeleteOps(opts SoftDeleteOptions) (bson.M, bson.M) {
	now := time.Now().UTC()
	state := opts.State
	if state == "" {
		state = "archived"
	}

	set := bson.M{
		"isDeleted": true,
		"state":     state,
		"deletedAt": now,
		"updatedAt": now, // สำคัญ
	}
	unset := bson.M{}

	if opts.ClearPhotos {
		set["photoKey"] = ""
		set["photoContentType"] = ""
		unset["photoOriginKey"] = 1
		unset["photoOriginContentType"] = 1
		unset["photoFaceKey"] = 1
		unset["photoFaceContentType"] = 1
	}

	for _, ns := range opts.ExternalNamespaces {
		set["external."+ns+".state"] = "deleted"
		set["external."+ns+".syncedAt"] = now
		// ล้าง id ที่ยึดโยง
		switch ns {
		case "iboc", "ibocdev":
			unset["external."+ns+".id"] = 1
			unset["external."+ns+".faceId"] = 1
		default:
			unset["external."+ns+".id"] = 1
		}
	}

	for k, v := range opts.ExtraSet {
		set[k] = v
	}
	for k, v := range opts.ExtraUnset {
		unset[k] = v
	}

	return set, unset
}

// SoftDeleteMany: อัปเดตเอกสารให้เป็น soft-delete ตาม filter
func SoftDeleteMany(ctx context.Context, collection string, filter bson.M, opts SoftDeleteOptions) (*mongo.UpdateResult, error) {
	set, unset := buildSoftDeleteOps(opts)
	return UpdateManyOps(ctx, collection, filter, set, unset)
}

func SoftDeleteOne(ctx context.Context, collection string, filter bson.M, opts SoftDeleteOptions) (*mongo.UpdateResult, error) {
    set, unset := buildSoftDeleteOps(opts)
    return UpdateOneOps(ctx, collection, filter, set, unset)
}

// ReviveMany: ใช้ตอน re-appear เพื่อปลุกให้ active
func ReviveMany(ctx context.Context, collection string, filter bson.M) (*mongo.UpdateResult, error) {
	set := bson.M{
		"isDeleted": false,
		"state":     "active",
		"updatedAt": time.Now().UTC(),
	}
	unset := bson.M{"deletedAt": 1}
	return UpdateManyOps(ctx, collection, filter, set, unset)
}
