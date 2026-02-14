package devsync

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"klynx/config"
	"klynx/internal/logger"
	"klynx/internal/services/atasvc"
	"klynx/models/devsyncmod"
	"klynx/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel"
)

func SyncDevicesAndChannelsByEdgeIDATA(ctx context.Context, edgeID string) (*devsyncmod.SyncResult, error) {
	tracer := otel.Tracer("klynx/devsync")
	ctx, span := tracer.Start(ctx, "DevSync.ATA")
	defer span.End()

	log := logger.FromCtx(ctx, "devsync", "ATA").With().Str("edgeId", edgeID).Logger()

	oid, err := primitive.ObjectIDFromHex(edgeID)
	if err != nil {
		return nil, fmt.Errorf("invalid id")
	}

	coll := config.MongoClient.Database("klynx").Collection("system_edges")

	var edge devsyncmod.ATAEdgeDoc
	err = coll.FindOne(ctx, bson.M{"_id": oid, "type": "ata"}).Decode(&edge)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			log.Warn().Msg("edge config not found")
			return nil, ErrEdgeNotFound
		}
		log.Error().Err(err).Msg("mongo findOne failed")
		return nil, fmt.Errorf("mongo findOne failed: %w", err)
	}

	// decrypt password (required)
	passwordPlain, err := utils.DecryptFromKeyringJSON(edge.PassEnc)
	if err != nil {
		log.Error().Err(err).Msg("decrypt password failed")
		return nil, err
	}

	// build BaseAPI url (ATA service expects full base api url)
	baseApiURL, err := url.Parse(strings.TrimRight(edge.URL, "/") + "/api/v1")
	if err != nil {
		return nil, fmt.Errorf("invalid edge url: %w", err)
	}

	// TLS behavior:
	// - edge.TLS=true => https (ถ้า URL มาเป็น http ต้อง normalize ฝั่ง data ให้ถูก)
	// - insecureTLS ควรเป็น config flag ไม่ใช่ตาม edge (แต่ถ้าจะผูก ก็ทำได้)
	insecureTLS := false

	ataClient, err := atasvc.NewClientFromConfig(atasvc.Config{
		BaseAPI:  baseApiURL,
		WSBase:   &url.URL{Scheme: "wss", Host: baseApiURL.Host},
		Username: edge.Username,
		Password: passwordPlain,

		// คุณบอก apiKey ไม่ต้อง decode: งั้นแค่ส่งผ่านเป็น string (ถ้าต้องใช้จริงในอนาคต)
		APIKey:    edge.ApiKey,
		APISecret: "", // ไม่ต้อง decode ตอนนี้
	}, insecureTLS)
	if err != nil {
		return nil, err
	}

	token, err := ataClient.Login(ctx)
	if err != nil {
		log.Error().Err(err).Msg("ata login failed")
		return nil, err
	}

	// ✅ ATA จริงๆ มี list devices แล้ว (ไม่ต้อง fake deviceId=1)
	devices, err := ataClient.GetDevices(ctx, token)
	if err != nil {
		log.Error().Err(err).Msg("get devices failed")
		return nil, err
	}

	cameraColl := config.MongoClient.Database("klynx").Collection("camera")
	inserted := 0
	updated := 0
	totalCh := 0
	perDeviceCounts := make(map[int64]int)

	for _, dev := range devices {
		channels, err := ataClient.GetChannelsByDevice(ctx, token, dev.ID)
		if err != nil {
			log.Error().Err(err).Int64("deviceId", dev.ID).Msg("get channels by device failed")
			continue
		}

		perDeviceCounts[dev.ID] = len(channels)
		totalCh += len(channels)

		for _, ch := range channels {
			doc, err := buildCameraDocFromChannel(
				&Client{cfg: Config{Username: edge.Username}},
				dev,
				ch,
			)
			if err != nil {
				continue
			}

			update := bson.M{
				"$set":         doc,
				"$setOnInsert": bson.M{"createdAt": time.Now().UTC()},
			}

			if ch.Latitude != "" && ch.Longitude != "" {
				if lat, err := strconv.ParseFloat(ch.Latitude, 64); err == nil {
					update["$set"].(map[string]any)["lat"] = lat
				}
				if lon, err := strconv.ParseFloat(ch.Longitude, 64); err == nil {
					update["$set"].(map[string]any)["lng"] = lon
				}
			}

			res, err := cameraColl.UpdateOne(
				ctx,
				bson.M{
					"ata.deviceId":  dev.ID,
					"ata.channelId": ch.ID,
				},
				update,
				options.Update().SetUpsert(true),
			)
			if err != nil {
				return nil, err
			}
			if res.MatchedCount == 0 && res.UpsertedCount == 1 {
				inserted++
			} else {
				updated++
			}
		}
	}

	log.Info().
		Int("devices", len(devices)).
		Int("channels", totalCh).
		Int("inserted", inserted).
		Int("updated", updated).
		Msg("ata sync finished")

	return &devsyncmod.SyncResult{
		Devices:         len(devices),
		Channels:        totalCh,
		Inserted:        inserted,
		Updated:         updated,
		PerDeviceCounts: perDeviceCounts,
	}, nil
}
