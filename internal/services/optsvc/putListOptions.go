// internal/services/optsvc/putListOptions.go
package optsvc

import (
	"context"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/optmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collOptions = "options" // เก็บทุก namespace ใน collection เดียว

// docID: ค่าเริ่มต้นของ kind = "list" → ได้ _id = "list.<ns>"
func docID(kind, ns string) string { return kind + "." + ns }

// UpsertOptions: upsert หลาย namespace จาก payload ที่ client ส่งมา
// เขียนเข้าเอกสาร _id = "<kind>.<ns>" โดย merge ฟิลด์ลง document ตรง ๆ
func UpsertOptions(ctx context.Context, kind string, p optmod.NamespacedOptionsPayload) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/optsvc", // tracerName
		"options.UpsertOptions",                  // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"optsvc", "UpsertOptions",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	col := config.DB.Collection(collOptions)
	now := time.Now().UTC().UnixMilli()

	for ns, content := range p {
		filter := bson.M{"_id": docID(kind, ns)}

		// รวมฟิลด์ที่ส่งมา + updatedAt
		setDoc := bson.M{}
		for k, v := range content {
			setDoc[k] = v
		}
		setDoc["updatedAt"] = now

		update := bson.M{
			"$set":         setDoc,
			"$setOnInsert": bson.M{"createdAt": now},
		}

		if _, err := col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil {
			log.Error().Err(err).Str("ns", ns).Str("kind", kind).Msg("upsert options failed")
			return err
		}
	}
	return nil
}
