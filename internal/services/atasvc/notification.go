// internal/services/atasvc/notification.go
package atasvc

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/aimodel"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func NotificationAllSummary(ctx context.Context, req aimodel.NotificationAllReq) (*aimodel.NotificationAllData, error) {
	log := logger.FromCtx(ctx, "atasvc", "NotificationAllSummary")

	start, end, err := parseDateRangeYYYYMMDD(req.DateTime)
	if err != nil {
		return nil, err
	}

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PerPage <= 0 {
		req.PerPage = 20
	}
	if req.PerPage > 200 {
		req.PerPage = 200
	}

	sortVal := int32(-1)
	if strings.ToLower(req.SortOrder) == "asc" {
		sortVal = 1
	}

	// ✅ เอาทุก event: ไม่ filter type, ไม่ filter isDeleted
	match := bson.M{
		"dateTimeCreate": bson.M{
			"$gte": start,
			"$lte": end,
		},
	}

	if req.SN != "" {
		match["sn"] = req.SN
	}
	if req.ChannelID > 0 {
		match["channelId"] = req.ChannelID
	}

	// ✅ search: address/zone/eventAttribute.name
	if s := strings.TrimSpace(req.Search); s != "" {
		if len(s) > 80 {
			s = s[:80]
		}

		pattern := s
		if _, err := regexp.Compile(pattern); err != nil {
			pattern = regexp.QuoteMeta(s)
		}

		match["$or"] = bson.A{
			bson.M{"address": bson.M{"$regex": pattern, "$options": "i"}},
			bson.M{"zone": bson.M{"$regex": pattern, "$options": "i"}},
			bson.M{"eventAttribute.name": bson.M{"$regex": pattern, "$options": "i"}},
		}
	}

	skip := int64((req.Page - 1) * req.PerPage)
	limit := int64(req.PerPage)

	// ✅ lookup camera เหมือนเดิม (เอา lat/lng/name มาประกบ)
	cameraLookup := bson.D{{Key: "$lookup", Value: bson.M{
		"from": "camera",
		"let": bson.M{
			"cid": "$channelId",
			"nm":  "$address",
		},
		"pipeline": bson.A{
			bson.M{"$match": bson.M{
				"$expr": bson.M{"$and": bson.A{
					bson.M{"$eq": bson.A{"$brand", "ATA"}},
					// NOTE: กล้องยังกรอง isDeleted ของ camera ได้ตามเดิม (อันนี้ไม่ใช่ event)
					bson.M{"$ne": bson.A{"$isDeleted", true}},
					bson.M{"$or": bson.A{
						bson.M{"$eq": bson.A{"$channel", "$$cid"}},
						bson.M{"$eq": bson.A{"$streamID", "$$cid"}},
						bson.M{"$eq": bson.A{"$ata.channelId", "$$cid"}},
						bson.M{"$eq": bson.A{"$name", "$$nm"}},
					}},
				}},
			}},
			bson.M{"$addFields": bson.M{
				"__prio": bson.M{"$cond": bson.A{
					bson.M{"$or": bson.A{
						bson.M{"$eq": bson.A{"$channel", "$$cid"}},
						bson.M{"$eq": bson.A{"$streamID", "$$cid"}},
						bson.M{"$eq": bson.A{"$ata.channelId", "$$cid"}},
					}},
					0, 1,
				}},
			}},
			bson.M{"$sort": bson.M{"__prio": 1}},
			bson.M{"$limit": 1},
			bson.M{"$project": bson.M{"_id": 0, "lat": 1, "lng": 1, "name": 1}},
		},
		"as": "cam",
	}}}

	cameraFlatten := bson.D{{Key: "$addFields", Value: bson.M{
		"camLat":  bson.M{"$ifNull": bson.A{bson.M{"$arrayElemAt": bson.A{"$cam.lat", 0}}, ""}},
		"camLng":  bson.M{"$ifNull": bson.A{bson.M{"$arrayElemAt": bson.A{"$cam.lng", 0}}, ""}},
		"camName": bson.M{"$ifNull": bson.A{bson.M{"$arrayElemAt": bson.A{"$cam.name", 0}}, "$address"}},
	}}}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: match}},
		cameraLookup,
		cameraFlatten,
		bson.D{{Key: "$facet", Value: bson.M{
			"data": bson.A{
				bson.M{"$sort": bson.M{"dateTimeCreate": sortVal}},
				bson.M{"$skip": skip},
				bson.M{"$limit": limit},
				bson.M{"$project": bson.M{
					"id":             bson.M{"$toString": "$_id"},
					"_id":            0,
					"sn":             1,
					"channelId":      1,
					"dateTimeCreate": 1,

					// imageUrl: array -> first
					"imageUrl0": bson.M{"$arrayElemAt": bson.A{"$imageUrl", 0}},

					// camera enrich
					"latitude":  "$camLat",
					"longitude": "$camLng",
					"address":   "$camName",

					// extras
					"zone":                  1,
					"type":                  1,
					"pictureCoordinates":    1,
					"eventAttributeDetails": 1,

					// eventAttribute
					"ea_name":            "$eventAttribute.name",
					"ea_listType":        "$eventAttribute.listType",
					"ea_similarity":      "$eventAttribute.similarity",
					"ea_idCard":          "$eventAttribute.idCard",
					"ea_featureImageUrl": "$eventAttribute.featureImageUrl",
				}},
			},

			// ✅ summary: group by "type" (เพราะเราเอาทุก event)
			"summary": bson.A{
				bson.M{"$group": bson.M{
					"_id":   "$type",
					"count": bson.M{"$sum": 1},
				}},
			},

			"total": bson.A{
				bson.M{"$count": "totalRecords"},
			},
		}}},
	}

	qctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var out []bson.M
	if err := stomongo.Aggregate(qctx, collectionATAEvents, pipeline, &out); err != nil {
		log.Error().Err(err).Msg("aggregate failed")
		return nil, err
	}

	resp := &aimodel.NotificationAllData{
		Details: []aimodel.BlacklistItem{},
		Pagination: aimodel.Pagination{
			Page:      req.Page,
			PerPage:  req.PerPage,
			SortField: "dateTimeCreate",
			SortOrder: strings.ToLower(req.SortOrder),
		},
		Summary: aimodel.NotificationAllSummary{
			Total:  0,
			ByType: []aimodel.NotificationTypeCount{},
		},
	}

	if len(out) == 0 {
		return resp, nil
	}
	root := out[0]

	// totalRecords
	totalRecords := int64(0)
	if arr, ok := root["total"].(bson.A); ok && len(arr) > 0 {
		if m, ok := arr[0].(bson.M); ok {
			totalRecords = toInt64(m["totalRecords"])
		}
	}
	resp.Pagination.TotalRecords = totalRecords
	if totalRecords > 0 {
		resp.Pagination.TotalPages = (totalRecords + int64(req.PerPage) - 1) / int64(req.PerPage)
	}

	// summary by type
	if arr, ok := root["summary"].(bson.A); ok && len(arr) > 0 {
		var sum int64 = 0
		for _, it := range arr {
			m, _ := it.(bson.M)
			t := strings.TrimSpace(fmt.Sprint(m["_id"]))
			cnt := toInt64(m["count"])
			sum += cnt
			resp.Summary.ByType = append(resp.Summary.ByType, aimodel.NotificationTypeCount{
				Type:  t,
				Count: cnt,
			})
		}
		resp.Summary.Total = sum
	}

	// data
	if arr, ok := root["data"].(bson.A); ok {
		for _, it := range arr {
			m, _ := it.(bson.M)

			img := ""
			p := strings.TrimSpace(fmt.Sprint(m["imageUrl0"]))
			if p != "" && p != "<nil>" {
				img = buildImageProxyURL(p)
			}

			lat := fmt.Sprint(m["latitude"])
			if lat == "<nil>" {
				lat = ""
			}
			lng := fmt.Sprint(m["longitude"])
			if lng == "<nil>" {
				lng = ""
			}

			resp.Details = append(resp.Details, aimodel.BlacklistItem{
				ID:        fmt.Sprint(m["id"]),
				SN:        fmt.Sprint(m["sn"]),
				ChannelID: toInt64(m["channelId"]),

				Timestamp: toRFC3339(m["dateTimeCreate"]),
				DateTime:  toTimeISO(m["dateTimeCreate"]),

				CameraName: fmt.Sprint(m["address"]),
				ImageUrl:   img,

				Lat: lat,
				Lng: lng,

				PictureCoordinates:    toAnySlice(m["pictureCoordinates"]),
				EventAttributeDetails: m["eventAttributeDetails"],
				Zone:                  fmt.Sprint(m["zone"]),
				Type:                  fmt.Sprint(m["type"]),

				// เดิมของ BlacklistItem (ยังคงเติมให้ได้)
				Name:            fmt.Sprint(m["ea_name"]),
				ListType:        m["ea_listType"],
				Similarity:      toFloat64(m["ea_similarity"]),
				IDCard:          fmt.Sprint(m["ea_idCard"]),
				FeatureImageUrl: fmt.Sprint(m["ea_featureImageUrl"]),
			})
		}
	}

	return resp, nil
}
