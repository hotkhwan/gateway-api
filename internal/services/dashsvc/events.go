// internal/services/dashsvc/events.go
package dashsvc

import (
	"context"
	"math"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/dashmod"
	"github.com/hotkhwan/gateway-api/utils"
	"github.com/hotkhwan/gateway-api/utils/aiutil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	collectionATAEvents = "ata_events"
)

// ---------------- internal struct สำหรับดึงจาก Mongo ----------------

type ataEvent struct {
	ID           primitive.ObjectID `bson:"_id"`
	Type         string             `bson:"type"`
	ChannelID    int64              `bson:"channelId"`
	EventDate    string             `bson:"eventDate"`
	Address      string             `bson:"address"`
	ImageURL     []string           `bson:"imageUrl"`
	VideoClipURL string             `bson:"videoClipUrl"`
	PictureURL   []string           `bson:"pictureUrl"`
	URL          string             `bson:"url"`

	IsDeleted    bool   `bson:"isDeleted"`
	Acknowledged bool   `bson:"acknowledged"`
	SOP          bson.M `bson:"sop"`

	EventAttr   ataEventAttr        `bson:"eventAttribute"`
	EventAttrDt ataEventAttrDetails `bson:"eventAttributeDetails"`
}

type ataEventAttr struct {
	ArrowEndName string  `bson:"arrowEndName"`
	ListType     int     `bson:"listType"`
	Similarity   float64 `bson:"similarity"` // 0.598 => 59.8%
	Name         string  `bson:"name"`
}

type ataEventAttrDetails struct {
	ListType   string  `bson:"listType"`
	Similarity float64 `bson:"similarity"`
}

// ใช้สำหรับ aggregate peopleCountingArea
type peopleCountingAgg struct {
	Total int `bson:"total"`
	In    int `bson:"in"`
	Out   int `bson:"out"`
}

// ---------------- public service function ----------------

func BuildAtaDashboard(ctx context.Context, start, end time.Time) (dashmod.DashboardDetails, error) {
	ctx, endSpan, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/evtsvc",
		"dashboard.BuildAtaDashboard",
		"evtsvc",
		"BuildAtaDashboard",
	)
	defer endSpan()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection(collectionATAEvents)

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	details := dashmod.DashboardDetails{
		Event: dashmod.DashboardEventStats{},
		Camera: dashmod.DashboardSimpleStats{
			Total:   0,
			Online:  0,
			Offline: 0,
		},
		Kcontrol: dashmod.DashboardSimpleStats{
			Total:   0,
			Online:  0,
			Offline: 0,
		},
		PeopleCounting: dashmod.DashboardPeopleCountingStats{},
		Notification:   []dashmod.DashboardNotificationItem{},
		Blacklist:      []dashmod.DashboardNotificationItem{},
	}

	// ---------------- 1) Event stats ----------------
	baseFilter := bson.M{
		"isDeleted": bson.M{"$ne": true},
		"dateTimeCreate": bson.M{
			"$gte": start,
			"$lt":  end,
		},
	}

	total, err := coll.CountDocuments(ctx, baseFilter)
	if err != nil {
		log.Error().Err(err).Msg("❌ failed to count total events")
		return details, err
	}

	vehicleFilter := bson.M{}
	for k, v := range baseFilter {
		vehicleFilter[k] = v
	}
	vehicleFilter["type"] = bson.M{"$regex": "Vehicle", "$options": "i"}

	vehicle, err := coll.CountDocuments(ctx, vehicleFilter)
	if err != nil {
		log.Error().Err(err).Msg("❌ failed to count vehicle events")
		return details, err
	}

	motorFilter := bson.M{}
	for k, v := range baseFilter {
		motorFilter[k] = v
	}
	motorFilter["type"] = "Motor Vehicle Capture"

	motorVehicle, err := coll.CountDocuments(ctx, motorFilter)
	if err != nil {
		log.Error().Err(err).Msg("❌ failed to count motorVehicle events")
		return details, err
	}

	faceFilter := bson.M{}
	for k, v := range baseFilter {
		faceFilter[k] = v
	}
	faceFilter["type"] = "Face Capture"

	faceCount, err := coll.CountDocuments(ctx, faceFilter)
	if err != nil {
		log.Error().Err(err).Msg("❌ failed to count face events")
		return details, err
	}

	noneVehicle := int(total) - int(vehicle)

	details.Event.Total = int(total)
	details.Event.Vehicle = int(vehicle)
	details.Event.Face = int(faceCount)
	details.Event.MotorVehicle = int(motorVehicle)
	details.Event.NoneVehicle = noneVehicle

	// ---------------- 2) PeopleCountingArea ----------------
	var pcAgg []peopleCountingAgg
	pcPipeline := mongoPeopleCountingPipeline(start, end)
	if err := stomongo.Aggregate(ctx, collectionATAEvents, pcPipeline, &pcAgg); err != nil {
		log.Error().Err(err).Msg("❌ failed to aggregate peopleCountingArea")
	} else if len(pcAgg) > 0 {
		details.PeopleCounting.Total = pcAgg[0].Total
		details.PeopleCounting.In = pcAgg[0].In
		details.PeopleCounting.Out = pcAgg[0].Out
	}

	// ---------------- 3) Notification + Blacklist ----------------
	const maxNotif = 5
	const maxBlacklist = 5

	// 3.a) Notification (ไม่ใช่ blacklist)
	func() {
		notifFilter := bson.M{
			"isDeleted":    bson.M{"$ne": true},
			"acknowledged": bson.M{"$ne": true},
			"dateTimeCreate": bson.M{
				"$gte": start,
				"$lt":  end,
			},
			"$or": []bson.M{
				{"type": bson.M{"$ne": "Face Capture"}},
				{"eventAttribute.listType": 0},
				{"eventAttribute.listType": bson.M{"$exists": false}},
			},
		}

		findOpts := options.Find().
			SetSort(bson.D{{Key: "dateTimeCreate", Value: -1}}).
			SetLimit(int64(maxNotif))

		cur, err := coll.Find(ctx, notifFilter, findOpts)
		if err != nil {
			log.Error().Err(err).Msg("❌ failed to query notification")
			return
		}
		defer cur.Close(ctx)

		for cur.Next(ctx) {
			var ev ataEvent
			if err := cur.Decode(&ev); err != nil {
				log.Error().Err(err).Msg("❌ decode notification failed")
				continue
			}

			item := buildNotificationItemFromEvent(ev)
			item.ID = ev.ID.Hex()

			// ✅ Notification: ไม่ต้อง lookup camera
			details.Notification = append(details.Notification, item)

			if len(details.Notification) >= maxNotif {
				break
			}
		}

		if err := cur.Err(); err != nil {
			log.Error().Err(err).Msg("❌ cursor error while reading ata_events for notification")
		}
	}()

	// 3.b) Blacklist: Face Capture + listType != 0 + ต้องยังไม่ ack + ยังไม่ deleted
	func() {
		blacklistFilter := bson.M{
			"isDeleted":    bson.M{"$ne": true},
			"acknowledged": bson.M{"$ne": true},
			"dateTimeCreate": bson.M{
				"$gte": start,
				"$lt":  end,
			},
			"type":                    "Face Capture",
			"eventAttribute.listType": bson.M{"$ne": 0},
		}

		findOpts := options.Find().
			SetSort(bson.D{{Key: "dateTimeCreate", Value: -1}}).
			SetLimit(int64(maxBlacklist))

		cur, err := coll.Find(ctx, blacklistFilter, findOpts)
		if err != nil {
			log.Error().Err(err).Msg("❌ failed to query recent events for blacklist")
			return
		}
		defer cur.Close(ctx)

		for cur.Next(ctx) {
			var ev ataEvent
			if err := cur.Decode(&ev); err != nil {
				log.Error().Err(err).Msg("❌ failed to decode ataEvent (blacklist)")
				continue
			}

			item := buildNotificationItemFromEvent(ev)
			item.ID = ev.ID.Hex()

			// ✅ lookup camera เฉพาะ blacklist
			camCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			cam, camErr := aiutil.FindCameraLatLong(camCtx, db, ev.ChannelID, ev.Address)
			cancel()

			if camErr != nil {
				log.Warn().Err(camErr).
					Str("address", ev.Address).
					Int64("channelId", ev.ChannelID).
					Msg("⚠️ camera lookup failed (blacklist)")
			} else if cam != nil {
				item.Lat = cam.Lat
				item.Long = cam.Long
			}

			details.Blacklist = append(details.Blacklist, item)

			if len(details.Blacklist) >= maxBlacklist {
				break
			}
		}

		if err := cur.Err(); err != nil {
			log.Error().Err(err).Msg("❌ cursor error while reading ata_events for blacklist")
		}
	}()

	// ---------------- 4) Camera stats ----------------
	func() {
		camColl := db.Collection("camera")

		// ✅ ไม่นับ isDeleted=true (รวมเคส field ไม่มีด้วย)
		base := bson.M{
			"$or": []bson.M{
				{"isDeleted": bson.M{"$exists": false}},
				{"isDeleted": false},
			},
		}

		total, err := camColl.CountDocuments(ctx, base)
		if err != nil {
			log.Error().Err(err).Msg("❌ failed to count cameras")
			return
		}

		onlineFilter := bson.M{
			"$and": bson.A{
				base,
				bson.M{"status": true},
			},
		}
		online, err := camColl.CountDocuments(ctx, onlineFilter)
		if err != nil {
			log.Error().Err(err).Msg("❌ failed to count online cameras")
			return
		}

		offlineFilter := bson.M{
			"$and": bson.A{
				base,
				bson.M{"$or": []bson.M{
					{"status": false},
					{"status": bson.M{"$exists": false}},
				}},
			},
		}
		offline, err := camColl.CountDocuments(ctx, offlineFilter)
		if err != nil {
			log.Error().Err(err).Msg("❌ failed to count offline cameras")
			return
		}

		details.Camera.Total = int(total)
		details.Camera.Online = int(online)
		details.Camera.Offline = int(offline)
	}()

	// ---------------- 5) Kcontrol stats ----------------
	func() {
		kcColl := db.Collection("kcontrol")

		total, err := kcColl.CountDocuments(ctx, bson.M{})
		if err != nil {
			log.Error().Err(err).Msg("❌ failed to count kcontrol devices")
			return
		}

		online, err := kcColl.CountDocuments(ctx, bson.M{"status": "online"})
		if err != nil {
			log.Error().Err(err).Msg("❌ failed to count online kcontrol devices")
			return
		}

		offline, err := kcColl.CountDocuments(ctx, bson.M{"status": "offline"})
		if err != nil {
			log.Error().Err(err).Msg("❌ failed to count offline kcontrol devices")
			return
		}

		details.Kcontrol.Total = int(total)
		details.Kcontrol.Online = int(online)
		details.Kcontrol.Offline = int(offline)
	}()

	log.Debug().
		Int("eventsTotal", details.Event.Total).
		Int("notifCount", len(details.Notification)).
		Int("blacklistCount", len(details.Blacklist)).
		Msg("✅ BuildAtaDashboard completed")

	return details, nil
}

// ---------------- helper functions ----------------

func mongoPeopleCountingPipeline(start, end time.Time) mongo.Pipeline {
	return mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{
			"isDeleted": bson.M{"$ne": true},
			"type":      "Pedestrian Traffic Statistics",
			"dateTimeCreate": bson.M{
				"$gte": start,
				"$lt":  end,
			},
		}}},
		bson.D{{Key: "$addFields", Value: bson.M{
			"direction": bson.M{
				"$switch": bson.M{
					"branches": bson.A{
						bson.M{"case": bson.M{"$in": bson.A{"in", "$regionNames"}}, "then": "in"},
						bson.M{"case": bson.M{"$in": bson.A{"out", "$regionNames"}}, "then": "out"},
					},
					"default": "unknown",
				},
			},
		}}},
		bson.D{{Key: "$group", Value: bson.M{
			"_id":   nil,
			"total": bson.M{"$sum": 1},
			"in":    bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$direction", "in"}}, 1, 0}}},
			"out":   bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$direction", "out"}}, 1, 0}}},
		}}},
	}
}

func buildImageProxyURL(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}

	imagePath := utils.GetFilesProxyPath() // เช่น "/image"

	// full URL -> path only
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		if u, err := url.Parse(p); err == nil {
			p = u.Path
		}
	}

	// already proxy
	if strings.HasPrefix(p, imagePath+"/") {
		return p
	}

	p = strings.TrimLeft(p, "/")
	return imagePath + "/" + p
}

func buildNotificationItemFromEvent(ev ataEvent) dashmod.DashboardNotificationItem {
	// ✅ prefer ata-feature/images ก่อนเสมอ
	image := ""

	// 1) หาใน pictureUrl[] ก่อน
	if len(ev.ImageURL) > 0 {
		image = ev.ImageURL[0]
	} else if len(ev.PictureURL) > 0 {
		image = ev.PictureURL[0]
	} else if ev.URL != "" {
		image = ev.URL
	}

	ntype := "nonVehicle"
	switch {
	case containsIgnoreCase(ev.Type, "Motor Vehicle Capture"):
		ntype = "vehicle"
	case containsIgnoreCase(ev.Type, "Vehicle"):
		ntype = "vehicle"
	case containsIgnoreCase(ev.Type, "Face Capture"):
		ntype = "people"
	case containsIgnoreCase(ev.Type, "Pedestrian"):
		ntype = "people"
	case containsIgnoreCase(ev.Type, "Smoking Detection"):
		ntype = "detection"
	}

	acc := 0.0
	if ev.EventAttr.Similarity > 0 {
		acc = math.Round(ev.EventAttr.Similarity*10000) / 100
	} else if ev.EventAttrDt.Similarity > 0 {
		acc = math.Round(ev.EventAttrDt.Similarity*10000) / 100
	}

	lt := ev.EventAttr.ListType
	ltLabel := ""
	if lt != 0 {
		ltLabel = listTypeLabel(lt)
	}
	if ev.EventAttrDt.ListType != "" {
		ltLabel = ev.EventAttrDt.ListType
	}
	vclip := buildVideoProxyURL(ev.VideoClipURL)
	if vclip == "" {
		vclip = "" // explicit (อ่านง่าย/กัน future refactor)
	}
	return dashmod.DashboardNotificationItem{
		ImageURL:      buildImageProxyURL(image), // จะได้ "/image/ata-feature/images/..."
		VideoClipURL:  vclip,
		Type:          ntype,
		Similarity:    acc,
		Name:          ev.EventAttr.Name,
		DeviceName:    ev.Address,
		Timestamp:     normalizeEventTime(ev.EventDate),
		ListType:      lt,
		ListTypeLabel: ltLabel,
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

func normalizeEventTime(s string) string {
	if s == "" {
		return ""
	}
	loc, _ := time.LoadLocation("Asia/Bangkok")
	if loc == nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, loc)
	if err != nil {
		return s
	}
	return t.Format(time.RFC3339)
}

func containsIgnoreCase(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func buildVideoProxyURL(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}

	videoPath := strings.TrimSpace(os.Getenv("FILES_PROXY_PATH")) // เช่น "/files"
	if videoPath == "" {
		// fallback: ส่ง path ตรง ๆ ไปก่อน
		return p
	}
	if !strings.HasPrefix(videoPath, "/") {
		videoPath = "/" + videoPath
	}
	videoPath = strings.TrimRight(videoPath, "/")

	// full URL -> path only
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		if u, err := url.Parse(p); err == nil {
			p = u.Path
		}
	}

	// already proxy
	if strings.HasPrefix(p, videoPath+"/") {
		return p
	}

	p = strings.TrimLeft(p, "/")
	return videoPath + "/" + p
}
