// internal/services/devsync/ataSync.go
package devsync

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"klynx/config"
	"klynx/models/aimodel"
	"klynx/models/devsyncmod"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Convenience สำหรับ handler: ใช้ client จาก config.ATA
func SyncDevicesAndChannelsDefault(ctx context.Context) (*devsyncmod.SyncResult, error) {
	c, err := NewClientFromEnv()
	if err != nil {
		return nil, err
	}
	return SyncDevicesAndChannels(ctx, c)
}

// SyncDevicesAndChannels ดึง devices ทั้งหมด + channels แล้ว upsert -> collection "camera"
func SyncDevicesAndChannels(ctx context.Context, c *Client) (*devsyncmod.SyncResult, error) {
	token, err := c.Login(ctx)
	if err != nil {
		return nil, fmt.Errorf("ATA login: %w", err)
	}

	devices, err := c.GetDevices(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("GetDevices: %w", err)
	}
	if len(devices) == 0 {
		return &devsyncmod.SyncResult{}, nil
	}

	coll := config.MongoClient.Database("klynx").Collection("camera")

	var (
		totalChannels int
		inserted      int
		updated       int
		perDev        = make(map[int64]int)
		sample        []map[string]any
	)

	for _, dev := range devices {
		channels, err := c.GetChannelsByDevice(ctx, token, dev.ID)
		if err != nil {
			// ไม่ fail ทั้ง batch แต่ log / return error ขึ้นไปก็ได้
			return nil, fmt.Errorf("GetChannels device %d: %w", dev.ID, err)
		}
		perDev[dev.ID] = len(channels)
		totalChannels += len(channels)

		for _, ch := range channels {
			doc, err := buildCameraDocFromChannel(c, dev, ch)
			if err != nil {
				// skip แชนเนลที่ field พัง
				continue
			}

			filter := bson.M{
				"ata.deviceId":  dev.ID,
				"ata.channelId": ch.ID,
			}

			update := bson.M{
				"$set":         doc,
				"$setOnInsert": bson.M{"createdAt": time.Now().UTC()},
			}

			opts := options.Update().SetUpsert(true)
			res, err := coll.UpdateOne(ctx, filter, update, opts)
			if err != nil {
				return nil, fmt.Errorf("update camera (device=%d ch=%d): %w", dev.ID, ch.ID, err)
			}
			if res.MatchedCount == 0 && res.UpsertedCount == 1 {
				inserted++
			} else {
				updated++
			}

			if len(sample) < 5 {
				sample = append(sample, doc)
			}
		}
	}

	return &devsyncmod.SyncResult{
		Devices:         len(devices),
		Channels:        totalChannels,
		Inserted:        inserted,
		Updated:         updated,
		PerDeviceCounts: perDev,
		ChannelsSample:  sample,
	}, nil
}

// ----- helpers -----

func buildCameraDocFromChannel(c *Client, dev aimodel.ATADevice, ch aimodel.ATAChannel) (map[string]any, error) {
	now := time.Now().UTC()

	if strings.TrimSpace(ch.MainUrl) == "" {
		return nil, fmt.Errorf("channel %d empty mainUrl", ch.ID)
	}

	// --- map ตาม schema camera เดิม ---
	doc := map[string]any{}

	// url / key
	doc["url"] = ch.MainUrl
	doc["key"] = ch.MainUrl // เดิม key = url

	// name / district / brand
	doc["name"] = firstNonEmpty(ch.Name, fmt.Sprintf("ATA-%d-%d", dev.ID, ch.ID))
	doc["district"] = firstNonEmpty(ch.Zone, dev.Name) // ใช้ zone เป็น district
	doc["brand"] = "ATA"                               // หรือ "EDGEAI" ถ้าอยาก

	// channel เดิมคุณเก็บ 1 ตลอด เราจะตาม pattern เดิม
	doc["channel"] = 1

	// เพิ่ม streamID = channel id จาก ATA
	doc["streamID"] = ch.ID
	doc["cameraID"] = ch.ID

	// gbid (ถ้ามี)
	doc["gbid"] = strings.TrimSpace(ch.GB28181ID)

	// // lat / long Mapping
	// var lat, lon float64
	// if v, err := parseFloatStringSafe(ch.Latitude); err == nil {
	// 	lat = v
	// }
	// if v, err := parseFloatStringSafe(ch.Longitude); err == nil {
	// 	lon = v
	// }
	// doc["lat"] = lat
	// doc["long"] = lon

	// state / status
	doc["state"] = "synced"
	doc["status"] = ch.Enable == 1

	// datetime
	doc["dateTimeCreate"] = now
	doc["dateTimeUpdate"] = now

	// ip จาก device.ip หรือ host ใน RTSP
	ip := strings.TrimSpace(dev.IP)
	if ip == "" {
		ip = extractIPFromURL(ch.MainUrl)
	}
	if ip != "" {
		doc["ip"] = ip
	}

	// เพิ่ม meta ATA แยกอีกชั้น
	meta := bson.M{
		"deviceId":   dev.ID,
		"deviceSn":   dev.SN,
		"deviceName": dev.Name,

		"channelId":   ch.ID,
		"channelName": ch.Name,
		"zone":        ch.Zone,
		"enable":      ch.Enable,
		"width":       ch.Width,
		"height":      ch.Height,
		"codec":       ch.Codec,
		"tasks":       ch.Tasks,
		"taskInfos":   ch.TaskInfos,
	}

	// เพิ่ม wss flv url ให้ frontend
	sn := strings.TrimSpace(ch.SN)
	if sn == "" {
		sn = strings.TrimSpace(dev.SN)
	}

	if sn != "" && ch.ID != 0 {
		if ws, err := c.BuildFLVWSURL(sn, ch.ID); err == nil {
			meta["wsFlvUrl"] = ws
			doc["ataWsFlvUrl"] = ws
		}
	}

	doc["ata"] = meta

	return doc, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseFloatStringSafe(s string) (float64, error) {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return 0, fmt.Errorf("empty")
	}
	ss = strings.ReplaceAll(ss, ",", "")
	return strconv.ParseFloat(ss, 64)
}

func extractIPFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		host := u.Host
		if i := strings.IndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}
		return host
	}

	// fallback regex ง่าย ๆ
	for _, part := range strings.Fields(raw) {
		if strings.Count(part, ".") == 3 {
			return part
		}
	}
	return ""
}
