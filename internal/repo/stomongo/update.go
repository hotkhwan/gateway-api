// internal/adapters/repo/stomongo/update.go

// ✅ UpdateOne → แค่ $set (+ เติม updatedAt ให้อัตโนมัติ)
// ใช้ดีสำหรับ PATCH ที่ “แก้บางฟิลด์” และ ไม่ต้องเพิ่ม/ลบฟิลด์พิเศษ และ ไม่เพิ่ม rev

// ✅ UpdateOneOps → $set และ/หรือ $unset (+ เติม updatedAt เมื่อมี $set)
// เหมาะกับ PUT ที่ “แทนทั้ง resource” และต้อง ลบฟิลด์ ที่ไม่ได้ส่งมา

// หมายเหตุ: ถ้า call ด้วย $unset อย่างเดียว ตอนนี้ฟังก์ชันคุณจะ ไม่ เติม updatedAt (เพราะมี $set เมื่อ len(set)>0 เท่านั้น) — ถ้าอยากให้ updatedAt เปลี่ยนแม้ทำ $unset ล้วน แนะนำใส่ set := bson.M{"updatedAt": time.Now().UTC()} ไปด้วย หรือทำ helper แยก UpdateOneUnset(...) ที่บังคับอัพเดตเวลา

// ✅ UpdateOneSetInc → $set + $inc (+ เติม updatedAt)
// ใช้เมื่ออัปเดตข้อมูลพร้อม เพิ่ม/ลดตัวนับ เช่น rev: +1 — ต้องการ atomic update

// แล้วใช้อะไรที่ไหน?
// สถานการณ์	แนะนำใช้
// HTTP PATCH (svc) – อัปเดตบางฟิลด์ + ต้องเพิ่ม rev	UpdateOneSetInc ← เพื่อไม่เจอปัญหา $inc ใต้ $set และได้ atomic
// HTTP PATCH (svc) – อัปเดตบางฟิลด์เฉย ๆ ไม่เพิ่ม rev	UpdateOne
// HTTP PUT (svc) – อัปเดตทั้งทรัพยากร + ลบฟิลด์ที่หายไป	UpdateOneOps ($set + $unset)
// Kafka handlers – เขียน metadata ภายนอก/สถานะ (ไม่ถือเป็น “แก้ไขเนื้อหา” ของผู้ใช้)	ส่วนใหญ่ใช้ UpdateOne (หรือ UpdateOneOps เมื่อมี $unset) และ ไม่จำเป็นต้อง rev+1

package stomongo

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func UpdateByID(ctx context.Context, collection string, id primitive.ObjectID, setFields bson.M) (*mongo.UpdateResult, error) {
	return coll(collection).UpdateByID(ctx, id, bson.M{"$set": nowSet(setFields)})
}

func UpdateOne(ctx context.Context, collection string, filter bson.M, setFields bson.M) (*mongo.UpdateResult, error) {
	return coll(collection).UpdateOne(ctx, filter, bson.M{"$set": nowSet(setFields)})
}

func UpdateMany(ctx context.Context, collection string, filter bson.M, setFields bson.M) (*mongo.UpdateResult, error) {
	return coll(collection).UpdateMany(ctx, filter, bson.M{"$set": nowSet(setFields)})
}

func UpdateOneOps(ctx context.Context, collection string, filter bson.M, set bson.M, unset bson.M) (*mongo.UpdateResult, error) {
	update := bson.M{}
	if len(set) > 0 {
		update["$set"] = nowSet(set)
	}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	return coll(collection).UpdateOne(ctx, filter, update)
}

func UpdateOneSetInc(ctx context.Context, collection string, filter bson.M, setFields bson.M, incFields bson.M) (*mongo.UpdateResult, error) {
	update := bson.M{
		"$set": nowSet(setFields), // เติม updatedAt ให้ด้วย
	}
	if len(incFields) > 0 {
		update["$inc"] = incFields
	}
	return coll(collection).UpdateOne(ctx, filter, update)
}

func UpdateByIDOps(ctx context.Context, collection string, id primitive.ObjectID, set bson.M, unset bson.M) (*mongo.UpdateResult, error) {
	return UpdateOneOps(ctx, collection, bson.M{"_id": id}, set, unset)
}

// ถ้าต้องการ raw เต็ม ๆ แบบส่ง operator เอง
func UpdateOneRaw(ctx context.Context, collection string, filter bson.M, update bson.M) (*mongo.UpdateResult, error) {
	return coll(collection).UpdateOne(ctx, filter, update)
}
