// internal/services/atasvc/mapper.go
package atasvc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/internal/repo/stos3minio"
	"github.com/hotkhwan/gateway-api/models/aimodel"

	"github.com/rs/zerolog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.opentelemetry.io/otel/trace"
)

// PersistATAEvent
// - generate _id ล่วงหน้า
// - Unmarshal evt.Data → dataMap (bson.M)
// - upload pictureBase64List → S3 → pictureUrl (ใช้ path images/<sn>/<id>/<idx>.jpg)
// - map eventAttribute → eventAttributeDetails
// - flatten field สำคัญขึ้นมาบน doc
// - insert ลง collection "ata_events"
// - เก็บ traceId จาก ctx
// - คืน doc ที่ insert แล้ว เพื่อนำไปใช้ส่ง MQTT (enriched payload)
func PersistATAEvent(ctx context.Context, evt aimodel.PusherEvent) (bson.M, error) {
	log := logger.FromCtx(ctx, "atasvc", "PersistATAEvent")

	log.Debug().
		Str("event", evt.Event).
		Int64("time_ms", evt.TimeMs).
		Msg("📥 PersistATAEvent start")

	// -------- 1) แปลง evt.Data → map --------
	if len(evt.Data) == 0 {
		log.Warn().Msg("↷ Skip persist: evt.Data empty")
		return nil, nil
	}

	var (
		rawAny  any
		dataMap bson.M
	)

	// ขั้นแรก parse ดูว่า data เป็น object หรือ string
	if err := json.Unmarshal(evt.Data, &rawAny); err != nil {
		log.Error().Err(err).Msg("❌ Unmarshal ATA evt.Data failed (outer)")
		return nil, err
	}

	switch v := rawAny.(type) {
	case map[string]any:
		// เคส data เป็น object ตรง ๆ
		dataMap = bson.M(v)

	case string:
		// เคส data เป็น string ที่ห่อ JSON → แตกชั้นอีกที
		if err := json.Unmarshal([]byte(v), &dataMap); err != nil {
			log.Error().Err(err).Msg("❌ Unmarshal ATA evt.Data failed (inner string)")
			return nil, err
		}
	default:
		log.Debug().Msgf("⚠️ ATA evt.Data is unsupported type: %T, skip persist", v)
		return nil, nil
	}

	// -------- 2) generate _id สำหรับ doc นี้ --------
	id := primitive.NewObjectID()
	// -------- (NEW) 2.5) lookup camera แล้ว merge lat/long ฯลฯ --------
	// เดา key จาก event:
	// - sn: dataMap["sn"]
	// - channelId: dataMap["channelId"]
	// - address: dataMap["address"] (บางทีเป็นชื่อ channel/cam)
	// - channelName: dataMap["channelName"] (ถ้ามี)
	sn := stringFromMap(dataMap, "sn")
	address := stringFromMap(dataMap, "address")
	channelName := stringFromMap(dataMap, "channelName")
	channelID := dataMap["channelId"]

	// ใช้ context แยกสำหรับ mongo lookup กันช้า
	camCtx, camCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer camCancel()

	cam, camErr := tryFindCameraATA(camCtx, sn, channelID, address, channelName)
	if camErr != nil {
		log.Warn().Err(camErr).Msg("⚠️ camera lookup failed; continue without camera merge")
	} else if cam != nil {
		// NOTE: ตอนนี้ doc ยังไม่ถูกสร้าง ให้ merge ลง dataMap ก่อน
		// แล้วเดี๋ยวตอนสร้าง doc ค่อย flatten เพิ่มอีกรอบ
		// dataMap["camera"] = bson.M{
		// 	"id":       cam["_id"],
		// 	"name":     cam["name"],
		// 	"ip":       cam["ip"],
		// 	"url":      cam["url"],
		// 	"district": cam["district"],
		// 	"channel":  cam["channel"],
		// 	"ata":      cam["ata"],
		// 	"wsFlvUrl": cam["ataWsFlvUrl"],
		// }
		// ✅ เติม latitude/longitude ถ้าของเดิมว่าง
		if strings.TrimSpace(fmt.Sprint(dataMap["latitude"])) == "" {
			if v, ok := cam["lat"]; ok {
				dataMap["latitude"] = v
			}
		}
		if strings.TrimSpace(fmt.Sprint(dataMap["longitude"])) == "" {
			if v, ok := cam["long"]; ok {
				dataMap["longitude"] = v
			}
		}
		log.Debug().Interface("cameraId", cam["_id"]).Msg("📸 camera matched and merged into raw")
	}

	// -------- 3) จัดการรูป pictureBase64List → S3 (ใช้ sn + _id เป็น path) --------
	if err := handlePictureBase64List(ctx, log, &dataMap, id); err != nil {
		// best-effort: log error แต่ยังไปต่อได้
		log.Error().Err(err).Msg("⚠️ handlePictureBase64List failed")
	} else {
		log.Debug().Msg("🖼️ handlePictureBase64List done")
	}

	// -------- 4) map eventAttribute → eventAttributeDetails --------
	if attrRaw, ok := dataMap["eventAttribute"]; ok {
		if attrMap, ok2 := attrRaw.(map[string]interface{}); ok2 {
			dataMap["eventAttributeDetails"] = buildEventAttributeDetails(attrMap)
			log.Debug().Msg("🔍 eventAttributeDetails mapped")
		}
	}
	// -------- 4.1) (NEW) upload videoUrl to S3 for listType 2/3 --------
	if err := handleVideoUrlUpload(ctx, log, &dataMap, id); err != nil {
		log.Warn().Err(err).Msg("⚠️ handleVideoUrlUpload failed (best-effort)")
	}
	// -------- 5) ดึง traceID จาก ctx (OTEL) --------
	span := trace.SpanFromContext(ctx)
	sc := span.SpanContext()
	traceID := sc.TraceID().String()

	// -------- 6) สร้าง doc แบบ flatten + raw --------
	doc := bson.M{
		"_id":            id,
		"event":          evt.Event,
		"channel":        evt.Channel,
		"timeMs":         evt.TimeMs,
		"dateTimeCreate": time.Now().UTC(),
		"isDeleted":      false,
		"source":         "ata",
		"traceId":        traceID,

		// flatten field ต่าง ๆ (ตามที่คุณใส่ไว้แล้ว)
		"sn":                 dataMap["sn"],
		"deviceId":           dataMap["deviceId"],
		"eventDate":          dataMap["eventDate"],
		"eventDateValue":     dataMap["eventDateValue"],
		"type":               dataMap["type"],
		"typeValue":          dataMap["typeValue"],
		"zone":               dataMap["zone"],
		"address":            dataMap["address"],
		"warnLevel":          dataMap["warnLevel"],
		"eventVideo":         dataMap["eventVideo"],
		"videoUrl":           dataMap["videoUrl"],
		"roiUrl":             dataMap["roiUrl"],
		"url":                dataMap["url"],
		"sourceUrl":          dataMap["source"],
		"channelId":          dataMap["channelId"],
		"deviceGb28181Id":    dataMap["deviceGb28181Id"],
		"channelGb28181Id":   dataMap["channelGb28181Id"],
		"regionNames":        dataMap["regionNames"],
		"regionRois":         dataMap["regionRois"],
		"long":               dataMap["longitude"],
		"lat":                dataMap["latitude"],
		"imageUrl":           dataMap["pictureUrl"],
		"videoClipUrl":       dataMap["videoClipUrl"],
		"pictureCoordinates": dataMap["pictureCoordinates"],
		"idValue":            dataMap["id"],

		"eventAttribute":        dataMap["eventAttribute"],
		"eventAttributeDetails": dataMap["eventAttributeDetails"],

		"raw": dataMap,
	}

	const coll = "ata_events"

	// ใช้ context แยกสำหรับ Mongo
	mongoCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Debug().Str("collection", coll).Msg("💾 Inserting ATA event into Mongo")

	if _, err := stomongo.InsertOne(mongoCtx, coll, doc); err != nil {
		log.Error().Err(err).Str("collection", coll).Msg("❌ Insert ATA event failed")
		return nil, err
	}

	log.Debug().
		Str("collection", coll).
		Str("traceId", traceID).
		Interface("sn", doc["sn"]).
		Interface("deviceId", doc["deviceId"]).
		Msg("✅ ATA event persisted to Mongo")

	return doc, nil
}

// -----------------------------------------------------------------------------
// 2.1 จัดการรูป: pictureBase64List → S3 → pictureUrl (ใช้ sn + _id เป็น path)
// -----------------------------------------------------------------------------
func handlePictureBase64List(
	ctx context.Context,
	log zerolog.Logger,
	dataMap *bson.M,
	id primitive.ObjectID, // ใช้สำหรับประกอบ path
) error {
	raw, ok := (*dataMap)["pictureBase64List"]
	if !ok {
		log.Debug().Msg("ℹ️ pictureBase64List not found; skip image upload")
		return nil
	}

	arr, ok := raw.([]interface{})
	if !ok || len(arr) == 0 {
		log.Debug().Msg("ℹ️ pictureBase64List is empty or invalid; remove field")
		delete(*dataMap, "pictureBase64List")
		return nil
	}

	// ใช้ env ให้แมตช์กับ .env (ถ้าไม่มีใช้ default "ata")
	bucket := strings.TrimSpace(os.Getenv("S3_BUCKET_ATA_EVENTS"))
	if bucket == "" {
		bucket = "ata"
	}

	// ✅ ดึง sn จาก dataMap
	sn := stringFromMap(*dataMap, "sn")
	if sn == "" {
		sn = "unknown"
	}
	sn = sanitizeKey(sn)

	log.Debug().
		Str("bucket", bucket).
		Str("sn", sn).
		Str("id", id.Hex()).
		Int("count", len(arr)).
		Msg("🖼️ Starting upload ATA images to S3")

	var pictureURLs []string

	for idx, v := range arr {
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" {
			continue
		}

		imgBytes, err := decodeBase64Image(s)
		if err != nil {
			// best-effort: ข้ามรูปนั้นไป
			log.Warn().Err(err).
				Int("index", idx).
				Msg("⚠️ decodeBase64Image failed, skip this image")
			continue
		}
		if len(imgBytes) == 0 {
			continue
		}

		// 🔥 ใช้ sn + _id + idx
		// ตัวอย่าง key: images/60038008431d6040/693680afdb922a1f38405def/0.jpg
		key := fmt.Sprintf("images/%s/%s/%d.jpg", sn, id.Hex(), idx)

		bucketKey := strings.TrimSpace(os.Getenv("S3_BUCKET_ATA_EVENTS"))
		if bucketKey == "" {
			bucketKey = "ata-feature"
		}
		// url, err := stos3minio.Upload(ctx, bucketKey, false /*PRIVATE*/, key, imgBytes, "image/jpeg")
		_, err = stos3minio.Upload(ctx, bucketKey, false /*PRIVATE*/, key, imgBytes, "image/jpeg")
		if err != nil {
			log.Error().Err(err).
				Str("bucketKey", bucketKey).
				Str("key", key).
				Int("bytes", len(imgBytes)).
				Msg("❌ Upload image to S3 failed")
			continue
		}

		log.Debug().
			Str("bucket", bucket).
			Str("key", key).
			Int("bytes", len(imgBytes)).
			Msg("⬆️ Uploaded ATA image to S3")

		// pictureURLs = append(pictureURLs, url)
		pictureURLs = append(pictureURLs, "/"+bucketKey+"/"+key)
	}

	// เก็บเป็น pictureUrl (array) และลบ base64 ทิ้ง
	if len(pictureURLs) > 0 {
		(*dataMap)["pictureUrl"] = pictureURLs
		log.Debug().
			Int("count", len(pictureURLs)).
			Msg("✅ pictureUrl list populated in dataMap")
	}
	delete(*dataMap, "pictureBase64List")

	return nil
}

func decodeBase64Image(src string) ([]byte, error) {
	// รองรับรูปแบบ data:image/jpeg;base64,xxxx
	s := strings.TrimSpace(src)
	if strings.HasPrefix(s, "data:") {
		if i := strings.Index(s, ","); i >= 0 {
			s = s[i+1:]
		}
	}
	return base64.StdEncoding.DecodeString(s)
}

// -----------------------------------------------------------------------------
// (NEW) Upload videoUrl → S3 (only when listType == 2 or 3)
// -----------------------------------------------------------------------------
func handleVideoUrlUpload(
	ctx context.Context,
	log zerolog.Logger,
	dataMap *bson.M,
	id primitive.ObjectID,
) error {
	// ต้องมี eventAttribute.listType เป็น 2 หรือ 3
	attrRaw, ok := (*dataMap)["eventAttribute"]
	if !ok || attrRaw == nil {
		return nil
	}
	attrMap, ok := attrRaw.(map[string]interface{})
	if !ok {
		return nil
	}

	lt := toInt(attrMap["listType"])

	allowed := parseIntSetFromEnv("ATA_VIDEO_CLIP")
	if len(allowed) == 0 {
		// env ไม่ได้เปิด → ไม่ทำ video clip
		return nil
	}

	if _, ok := allowed[lt]; !ok {
		return nil
	}

	// ต้องมี videoUrl
	videoURL := strings.TrimSpace(fmt.Sprint((*dataMap)["videoUrl"]))
	if videoURL == "" || videoURL == "<nil>" {
		return nil
	}

	// กันทำซ้ำถ้ามีแล้ว
	if v := strings.TrimSpace(fmt.Sprint((*dataMap)["videoClipUrl"])); v != "" && v != "<nil>" {
		return nil
	}

	// ถ้าเป็น path /api/... ต้องมี base URL
	resolved := videoURL
	if strings.HasPrefix(videoURL, "/") {
		base := strings.TrimSpace(os.Getenv("ATA_API_URL"))
		base = strings.TrimRight(base, "/")
		if base == "" {
			log.Warn().Str("videoUrl", videoURL).Msg("ATA_API_URL is empty; cannot fetch relative videoUrl")
			return nil
		}

		// ✅ ตัด /api/v1 ที่ซ้ำกัน
		// ตัวอย่าง:
		// base:    https://ata-edge.k-lynx.com/api/v1
		// videoURL:/api/v1/publicvideos/xxx.mp4
		// => https://ata-edge.k-lynx.com/api/v1/publicvideos/xxx.mp4
		if strings.HasSuffix(base, "/api/v1") && strings.HasPrefix(videoURL, "/api/v1/") {
			resolved = base + strings.TrimPrefix(videoURL, "/api/v1")
		} else {
			resolved = base + videoURL
		}
	}

	// ตั้ง bucketKey (ใช้ตัวเดียวกับรูปเพื่อให้สอดคล้องกัน)
	bucketKey := strings.TrimSpace(os.Getenv("S3_BUCKET_ATA_EVENTS"))
	if bucketKey == "" {
		bucketKey = "ata-feature"
	}

	// ใช้ sn + mongo id ทำ path
	sn := stringFromMap(*dataMap, "sn")
	if sn == "" {
		sn = "unknown"
	}
	sn = sanitizeKey(sn)

	key := fmt.Sprintf("videoClip/%s/%s/0.mp4", sn, id.Hex())

	// HTTP GET (best-effort) + จำกัดขนาดกัน OOM
	reqCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, resolved, nil)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: 25 * time.Second,
	}
	log.Info().Str("resolved", resolved).Msg("🎥 fetching ATA video")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fetch video failed: status=%d", resp.StatusCode)
	}

	// limit 80MB (ปรับได้)
	const maxBytes = int64(80 * 1024 * 1024)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > maxBytes {
		return fmt.Errorf("video too large: %d bytes (limit %d)", len(body), maxBytes)
	}
	if len(body) == 0 {
		return fmt.Errorf("video body empty")
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "video/mp4"
	}

	// upload
	_, err = stos3minio.Upload(reqCtx, bucketKey, false /*PRIVATE*/, key, body, contentType)
	if err != nil {
		return err
	}

	// เก็บเป็น path (เหมือนรูป)
	(*dataMap)["videoClipUrl"] = "/" + bucketKey + "/" + key

	log.Debug().
		Int("bytes", len(body)).
		Str("bucketKey", bucketKey).
		Str("key", key).
		Str("listType", fmt.Sprint(lt)).
		Msg("🎬 Uploaded ATA video to S3")

	return nil
}

// -----------------------------------------------------------------------------
// utilities สำหรับ map / key
// -----------------------------------------------------------------------------

func sanitizeKey(s string) string {
	if s == "" {
		return "unknown"
	}
	// กันพวก space หรือ char แปลก ๆ
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "..", "_")
	return s
}

func stringFromMap(m bson.M, key string) string {
	if v, ok := m[key]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// -----------------------------------------------------------------------------
// 3. mapping eventAttribute → eventAttributeDetails
// -----------------------------------------------------------------------------

func buildEventAttributeDetails(attr map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})

	for k, v := range attr {
		code := toInt(v)

		switch k {
		// -------- Facial attributes --------
		case "age":
			out[k] = facialAgeLabel(code)
		case "gender":
			out[k] = genderLabel(code)
		case "mask":
			out[k] = maskLabel(code)
		case "glasses":
			out[k] = glassesLabel(code)
		case "listType":
			out[k] = listTypeLabel(code)

		// -------- Human attributes --------
		case "upper":
			out[k] = upperClothLabel(code)
		case "upperColor":
			out[k] = colorLabel(code)
		case "lower":
			out[k] = lowerClothLabel(code)
		case "lowerColor":
			out[k] = colorLabel(code)
		case "skirt":
			out[k] = skirtLabel(code)
		case "hat":
			out[k] = hatLabel(code)
		case "backPack":
			out[k] = backPackLabel(code)
		case "riding":
			out[k] = ridingLabel(code)
		case "direction":
			out[k] = directionLabel(code)
		case "hair":
			out[k] = hairLabel(code)
		case "upperTexture":
			out[k] = upperTextureLabel(code)
		case "shoe":
			out[k] = shoeLabel(code)
		case "shoeColor":
			out[k] = colorLabel(code)

		// -------- Non-motor Vehicle attributes --------
		case "helmet":
			out[k] = helmetLabel(code)
		case "number":
			out[k] = nonVehicleNumberLabel(code)
		case "nonVehicleType":
			out[k] = nonVehicleTypeLabel(code)
		case "nonvehicleColor":
			out[k] = colorLabel(code)

		// -------- Vehicle attributes --------
		case "plateColor":
			out[k] = plateColorLabel(code)
		case "carType":
			out[k] = carTypeLabel(code)
		case "carColor":
			out[k] = colorLabel(code)
		case "listTypeVehicle":
			out[k] = vehicleListTypeLabel(code)
		case "arrowDirection":
			out[k] = arrowDirectionLabel(code)

		default:
			// ถ้าไม่รู้จัก field ให้เก็บค่าดิบเอาไว้
			out[k] = v
		}
	}

	return out
}

func toInt(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float32:
		return int(t)
	case float64:
		return int(t)
	case string:
		var x int
		fmt.Sscanf(t, "%d", &x)
		return x
	default:
		return 0
	}
}

// ---------------- facial / human / vehicle code → label ----------------

func facialAgeLabel(code int) string {
	switch code {
	case 1:
		return "Child"
	case 2:
		return "Youth"
	case 3:
		return "Middle age"
	case 4:
		return "Old Age"
	default:
		return "Unknown"
	}
}

func genderLabel(code int) string {
	switch code {
	case 1:
		return "Male"
	case 2:
		return "Female"
	default:
		return "Unknown"
	}
}

func maskLabel(code int) string {
	switch code {
	case 1:
		return "Wear a mask"
	case 2:
		return "Not wearing a mask"
	default:
		return "Unknown"
	}
}

func glassesLabel(code int) string {
	switch code {
	case 1:
		return "Wear glasses"
	case 2:
		return "Not wearing glasses"
	default:
		return "Unknown"
	}
}

func listTypeLabel(code int) string {
	switch code {
	case 1:
		return "Whitelist"
	case 2:
		return "Red List"
	case 3:
		return "Blacklist"
	default:
		return "Strangers"
	}
}

// Human
func upperClothLabel(code int) string {
	switch code {
	case 1:
		return "Short Sleeves"
	case 2:
		return "Long Sleeves"
	default:
		return "Unknown"
	}
}

func lowerClothLabel(code int) string {
	switch code {
	case 1:
		return "Shorts"
	case 2:
		return "Long Pants"
	default:
		return "Unknown"
	}
}

func skirtLabel(code int) string {
	switch code {
	case 1:
		return "Wearing Skirt"
	case 2:
		return "Not Wearing Skirt"
	default:
		return "Unknown"
	}
}

func hatLabel(code int) string {
	switch code {
	case 1:
		return "Wearing Hat"
	case 2:
		return "Not Wearing Hat"
	default:
		return "Unknown"
	}
}

func backPackLabel(code int) string {
	switch code {
	case 1:
		return "Backpack"
	case 2:
		return "Shoulder Bag"
	case 3:
		return "Handbag"
	case 4:
		return "No Bag"
	default:
		return "Unknown"
	}
}

func ridingLabel(code int) string {
	switch code {
	case 1:
		return "Riding"
	case 2:
		return "Not Riding"
	default:
		return "Unknown"
	}
}

func directionLabel(code int) string {
	switch code {
	case 1:
		return "Front"
	case 2:
		return "Side"
	case 3:
		return "Back"
	default:
		return "Unknown"
	}
}

func hairLabel(code int) string {
	switch code {
	case 1:
		return "Short Hair"
	case 2:
		return "Long Hair"
	case 3:
		return "Updo"
	case 4:
		return "Bald"
	case 5:
		return "Medium-Length Hair"
	default:
		return "Unknown"
	}
}

func upperTextureLabel(code int) string {
	switch code {
	case 1:
		return "Plaid"
	case 2:
		return "Floral Print"
	case 3:
		return "Solid Color"
	case 4:
		return "Stripes"
	default:
		return "Unknown"
	}
}

func shoeLabel(code int) string {
	switch code {
	case 1:
		return "Leather Shoes"
	case 2:
		return "Sandals"
	case 3:
		return "Casual Shoes"
	case 4:
		return "Knee-High Boots"
	default:
		return "Unknown"
	}
}

// Non-motor
func helmetLabel(code int) string {
	switch code {
	case 1:
		return "Wearing a Helmet"
	case 2:
		return "Not Wearing a Helmet"
	default:
		return "Unknown"
	}
}

func nonVehicleNumberLabel(code int) string {
	switch code {
	case 1:
		return "1 Person"
	case 2:
		return "2 Persons"
	case 3:
		return "3 or More Persons"
	default:
		return "Unknown"
	}
}

func nonVehicleTypeLabel(code int) string {
	switch code {
	case 1:
		return "Motorcycle"
	case 2:
		return "Bicycle"
	case 3:
		return "Tricycle"
	default:
		return "Unknown"
	}
}

// Vehicle
func plateColorLabel(code int) string {
	switch code {
	case 1:
		return "Black"
	case 3:
		return "Blue"
	case 4:
		return "Green"
	case 10:
		return "White"
	case 11:
		return "Yellow"
	default:
		return "Unknown"
	}
}

func carTypeLabel(code int) string {
	switch code {
	case 1:
		return "Bus"
	case 2:
		return "Sedan"
	case 3:
		return "Heavy Truck"
	case 4:
		return "Pickup Truck"
	case 5:
		return "Engineering Vehicle"
	case 6:
		return "Box Truck"
	case 7:
		return "SUV"
	case 8:
		return "Passenger Car"
	case 9:
		return "Coach"
	case 10:
		return "Truck"
	case 11:
		return "Light Truck"
	case 12:
		return "MPV"
	case 13:
		return "Van"
	case 14:
		return "Off-Road Vehicle"
	case 15:
		return "Muck Truck"
	case 16:
		return "Concrete Mixer Truck"
	case 17:
		return "Crane Truck"
	case 18:
		return "Pump Truck"
	case 19:
		return "Sanitation Transport Vehicle"
	case 20:
		return "Silt Transport Vehicle"
	case 21:
		return "Ambulance"
	case 22:
		return "Emergency Command Vehicle"
	case 23:
		return "Fire Truck"
	case 24:
		return "Police Car"
	default:
		return "Unknown"
	}
}

func vehicleListTypeLabel(code int) string {
	switch code {
	case 1:
		return "Whitelist"
	case 2:
		return "Redlist"
	case 3:
		return "Blacklist"
	default:
		return "Unrecognized Plate"
	}
}

func arrowDirectionLabel(code int) string {
	switch code {
	case 1:
		return "Forward"
	case 2:
		return "Reverse"
	default:
		return "Unknown"
	}
}

// color ใช้ร่วมกัน (human, non-vehicle, vehicle)
func colorLabel(code int) string {
	switch code {
	case 1:
		return "Black"
	case 2:
		return "Brown"
	case 3:
		return "Blue"
	case 4:
		return "Green"
	case 5:
		return "Gray"
	case 6:
		return "Orange"
	case 7:
		return "Pink"
	case 8:
		return "Purple"
	case 9:
		return "Red"
	case 10:
		return "White"
	case 11:
		return "Yellow"
	default:
		return "Unknown"
	}
}

// -----------------------------------------------------------------------------
// Camera lookup + merge
// -----------------------------------------------------------------------------

// tryFindCameraATA: ค้นหา camera ที่เกี่ยวกับ ATA event โดยพยายาม match หลายแบบ
func tryFindCameraATA(ctx context.Context, sn string, channelID any, address string, channelName string) (bson.M, error) {
	const coll = "camera"

	toInt64 := func(v any) (int64, bool) {
		switch t := v.(type) {
		case int64:
			return t, true
		case int32:
			return int64(t), true
		case int:
			return int64(t), true
		case float64:
			return int64(t), true
		case float32:
			return int64(t), true
		case string:
			var x int64
			_, _ = fmt.Sscanf(strings.TrimSpace(t), "%d", &x)
			if x != 0 {
				return x, true
			}
			return 0, false
		default:
			return 0, false
		}
	}

	// helper เรียก FindOne แล้วคืน cam
	findOne := func(filter bson.M) (bson.M, error) {
		var cam bson.M
		if err := stomongo.FindOne(ctx, coll, filter, &cam); err != nil {
			return nil, err
		}
		// ถ้า decode ได้ แต่เอกสารว่าง
		if len(cam) == 0 {
			return nil, nil
		}
		return cam, nil
	}

	// 1) แม่นสุด: ata.deviceSn + ata.channelId
	if ch, ok := toInt64(channelID); ok {
		filter := bson.M{
			"brand": "ATA",
			"$or": []bson.M{
				{"channel": ch},
				{"streamID": ch},
				{"ata.channelId": ch},
			},
		}
		var cam bson.M
		if err := stomongo.FindOne(ctx, coll, filter, &cam); err == nil && len(cam) > 0 {
			return cam, nil
		}
	}

	// 2) รองลงมา: ata.deviceSn + name
	if sn != "" && (address != "" || channelName != "") {
		nameGuess := address
		if nameGuess == "" {
			nameGuess = channelName
		}
		filter := bson.M{
			"brand":        "ATA",
			"ata.deviceSn": sn,
			"name":         nameGuess,
		}
		if cam, err := findOne(filter); err == nil && cam != nil {
			return cam, nil
		}
	}

	// 3) เผื่อสุดท้าย: name อย่างเดียว
	// 2) ตาม requirement: match address -> camera.name (หลัก)
	if address != "" {
		addr := strings.TrimSpace(address)

		// exact match
		filter := bson.M{
			"brand": "ATA",
			"name":  addr,
		}
		if cam, err := findOne(filter); err == nil && cam != nil {
			return cam, nil
		}

		// normalized match (กัน space/underscore/case)
		normalize := func(s string) string {
			s = strings.TrimSpace(strings.ToLower(s))
			s = strings.ReplaceAll(s, " ", "")
			s = strings.ReplaceAll(s, "_", "")
			return s
		}
		addrN := normalize(addr)

		filter2 := bson.M{
			"brand": "ATA",
			"$expr": bson.M{
				"$eq": []any{
					bson.M{
						"$replaceAll": bson.M{
							"input": bson.M{
								"$replaceAll": bson.M{
									"input":       bson.M{"$toLower": "$name"},
									"find":        " ",
									"replacement": "",
								},
							},
							"find":        "_",
							"replacement": "",
						},
					},
					addrN,
				},
			},
		}
		if cam, err := findOne(filter2); err == nil && cam != nil {
			return cam, nil
		}
	}

	return nil, nil
}
