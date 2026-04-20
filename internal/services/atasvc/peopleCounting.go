// internal/services/atasvc/peopleCounting.go
package atasvc

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/aimodel"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	collectionATAEvents   = "ata_events"
	peopleCountingTypeStr = "Pedestrian Traffic Statistics"
)

// ✅ marker error for 400
var ErrBadRequest = errors.New("bad request")

func badReq(msg string, cause error) error {
	if cause == nil {
		return fmt.Errorf("%w: %s", ErrBadRequest, msg)
	}
	return fmt.Errorf("%w: %s: %v", ErrBadRequest, msg, cause)
}

// ✅ parse both "YYYY-MM-DD,YYYY-MM-DD" and "RFC3339,RFC3339"
func parseDateRangeFlexible(s string) (time.Time, time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, time.Time{}, badReq("dateTime is required", nil)
	}
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return time.Time{}, time.Time{}, badReq("dateTime must be 'start,end'", nil)
	}

	a := strings.TrimSpace(parts[0])
	b := strings.TrimSpace(parts[1])

	parseOne := func(x string, isEnd bool) (time.Time, error) {
		// heuristic: RFC3339 has T or Z or +hh:mm
		if strings.Contains(x, "T") || strings.Contains(x, "Z") || strings.Contains(x, "+") {
			t, err := time.Parse(time.RFC3339, x)
			if err != nil {
				return time.Time{}, err
			}
			return t.UTC(), nil
		}

		// YYYY-MM-DD
		t, err := time.Parse("2006-01-02", x)
		if err != nil {
			return time.Time{}, err
		}
		t = t.UTC()
		if isEnd {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		return t, nil
	}

	start, err := parseOne(a, false)
	if err != nil {
		return time.Time{}, time.Time{}, badReq("invalid dateTime start", err)
	}
	end, err := parseOne(b, true)
	if err != nil {
		return time.Time{}, time.Time{}, badReq("invalid dateTime end", err)
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, badReq("dateTime end must be >= start", nil)
	}
	return start, end, nil
}

func PeopleCountingSummary(ctx context.Context, req aimodel.PeopleCountingSummaryReq) (*aimodel.PeopleCountingSummaryResponse, error) {
	start, end, err := parseDateRangeFlexible(req.DateTime)
	if err != nil {
		return nil, err
	}

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PerPage <= 0 {
		req.PerPage = 10
	}
	if req.PerPage > 200 {
		req.PerPage = 200
	}

	sortVal := int32(-1)
	if strings.ToLower(req.SortOrder) == "asc" {
		sortVal = 1
	}

	// -----------------------------
	// Base match (index-friendly)
	// -----------------------------
	match := bson.M{
		"isDeleted": bson.M{"$ne": true},
		"type":      peopleCountingTypeStr,
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

	// search by address/zone regex (limit pattern length)
	if s := strings.TrimSpace(req.Search); s != "" {
		pattern := s
		if len(pattern) > 80 {
			pattern = pattern[:80]
		}
		if _, err := regexp.Compile(pattern); err != nil {
			pattern = regexp.QuoteMeta(pattern)
		}
		match["$or"] = bson.A{
			bson.M{"address": bson.M{"$regex": pattern, "$options": "i"}},
			bson.M{"zone": bson.M{"$regex": pattern, "$options": "i"}},
		}
	}

	// pagination
	skip := int64((req.Page - 1) * req.PerPage)
	limit := int64(req.PerPage)

	// -----------------------------
	// add direction from regionNames
	// -----------------------------
	addDirection := bson.D{{Key: "$addFields", Value: bson.M{
		"direction": bson.M{
			"$switch": bson.M{
				"branches": bson.A{
					bson.M{"case": bson.M{"$in": bson.A{"in", "$regionNames"}}, "then": "in"},
					bson.M{"case": bson.M{"$in": bson.A{"out", "$regionNames"}}, "then": "out"},
				},
				"default": "unknown",
			},
		},
	}}}

	// -----------------------------
	// direction filter (supports: DirectionCode / Directions[] / Direction regex)
	// -----------------------------
	directionMatch := bson.D{}

	dirSet := make([]string, 0, 2)
	seenDir := map[string]struct{}{}
	for _, d := range req.Directions {
		dd := strings.ToLower(strings.TrimSpace(d))
		if dd != "in" && dd != "out" {
			continue
		}
		if _, ok := seenDir[dd]; ok {
			continue
		}
		seenDir[dd] = struct{}{}
		dirSet = append(dirSet, dd)
	}

	switch req.DirectionCode {
	case 1:
		directionMatch = append(directionMatch, bson.E{Key: "direction", Value: "in"})
	case 2:
		directionMatch = append(directionMatch, bson.E{Key: "direction", Value: "out"})
	default:
		if len(dirSet) > 0 {
			directionMatch = append(directionMatch, bson.E{
				Key:   "direction",
				Value: bson.M{"$in": dirSet},
			})
		} else if s := strings.TrimSpace(req.Direction); s != "" {
			// backward compat: regex
			pattern := s
			if len(pattern) > 32 {
				pattern = pattern[:32]
			}
			if _, err := regexp.Compile(pattern); err != nil {
				pattern = regexp.QuoteMeta(pattern)
			}
			directionMatch = append(directionMatch, bson.E{
				Key: "direction",
				Value: bson.M{
					"$regex":   pattern,
					"$options": "i",
				},
			})
		}
	}

	// -----------------------------
	// camera multi-select
	// numeric -> filter early by channelId (fast)
	// name    -> filter later after lookup/flatten via camName
	// -----------------------------
	var numericChIDs []int64
	var cameraNames []string
	for _, c := range req.Cameras {
		cc := strings.TrimSpace(c)
		if cc == "" {
			continue
		}
		if v, err := strconv.ParseInt(cc, 10, 64); err == nil && v > 0 {
			numericChIDs = append(numericChIDs, v)
		} else {
			cameraNames = append(cameraNames, cc)
		}
	}

	// numeric camera ids -> match early by channelId (index-friendly),
	// but don't override explicit ChannelID param
	if len(numericChIDs) > 0 && req.ChannelID <= 0 {
		match["channelId"] = bson.M{"$in": numericChIDs}
	}

	cameraNameMatch := bson.D{}
	if len(cameraNames) > 0 {
		cameraNameMatch = append(cameraNameMatch, bson.E{
			Key:   "camName",
			Value: bson.M{"$in": cameraNames},
		})
	}

	// -----------------------------
	// lookup camera (ONLY for facet.data, AFTER skip/limit)
	// -----------------------------
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

	// -----------------------------
	// Build pipeline
	// IMPORTANT: lookup moved into facet.data only
	// -----------------------------
	base := mongo.Pipeline{
		bson.D{{Key: "$match", Value: match}},
		addDirection,
	}
	if len(directionMatch) > 0 {
		base = append(base, bson.D{{Key: "$match", Value: directionMatch}})
	}

	// facet.data pipeline: sort/skip/limit first (cheap), then lookup (only for page items)
	dataPipe := bson.A{
		bson.M{"$sort": bson.M{"dateTimeCreate": sortVal}},
		bson.M{"$skip": skip},
		bson.M{"$limit": limit},

		// only now we lookup + flatten (applies only to the page items)
		cameraLookup,
		cameraFlatten,
	}
	if len(cameraNameMatch) > 0 {
		dataPipe = append(dataPipe, bson.D{{Key: "$match", Value: cameraNameMatch}})
	}

	dataPipe = append(dataPipe,
		bson.M{"$project": bson.M{
			"id":             bson.M{"$toString": "$_id"},
			"_id":            0,
			"sn":             1,
			"channelId":      1,
			"dateTimeCreate": 1,
			"regionNames":    1,
			"direction":      1,
			"imageUrl0":      bson.M{"$arrayElemAt": bson.A{"$imageUrl", 0}},

			"latitude":  "$camLat",
			"longitude": "$camLng",
			"address":   "$camName",

			"regionRois":            1,
			"pictureCoordinates":    1,
			"zone":                  1,
			"type":                  1,
			"eventAttributeDetails": 1,
		}},
	)

	pipeline := append(base,
		bson.D{{Key: "$facet", Value: bson.M{
			"data": dataPipe,

			// ✅ summary does NOT do lookup anymore
			"summary": bson.A{
				bson.M{"$group": bson.M{
					"_id":   nil,
					"total": bson.M{"$sum": 1},
					"in": bson.M{"$sum": bson.M{"$cond": bson.A{
						bson.M{"$eq": bson.A{"$direction", "in"}}, 1, 0,
					}}},
					"out": bson.M{"$sum": bson.M{"$cond": bson.A{
						bson.M{"$eq": bson.A{"$direction", "out"}}, 1, 0,
					}}},
				}},
			},

			// ✅ total count does NOT do lookup anymore
			"total": bson.A{bson.M{"$count": "totalRecords"}},
		}}},
	)

	// -----------------------------
	// Execute
	// -----------------------------
	// 8s is too aggressive for aggregation + network; bump to reduce 500s under load
	qctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var out []bson.M
	if err := stomongo.Aggregate(qctx, collectionATAEvents, pipeline, &out); err != nil {
		return nil, err
	}

	resp := &aimodel.PeopleCountingSummaryResponse{
		Details: []aimodel.PeopleCountingItem{},
		Pagination: aimodel.Pagination{
			Page: req.Page, PerPage: req.PerPage,
			SortField: "dateTimeCreate", SortOrder: strings.ToLower(req.SortOrder),
		},
		Summary: aimodel.PeopleCountingSummary{},
		Status:  true,
	}
	if len(out) == 0 {
		return resp, nil
	}

	root := out[0]

	// totalRecords
	if arr, ok := root["total"].(bson.A); ok && len(arr) > 0 {
		if m, ok := arr[0].(bson.M); ok {
			resp.Pagination.TotalRecords = toInt64(m["totalRecords"])
		}
	}
	if resp.Pagination.TotalRecords > 0 {
		resp.Pagination.TotalPages = (resp.Pagination.TotalRecords + int64(req.PerPage) - 1) / int64(req.PerPage)
	}

	// summary
	if arr, ok := root["summary"].(bson.A); ok && len(arr) > 0 {
		if m, ok := arr[0].(bson.M); ok {
			resp.Summary.Total = toInt64(m["total"])
			resp.Summary.In = toInt64(m["in"])
			resp.Summary.Out = toInt64(m["out"])
		}
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

			// timestamp (string) for UI
			ts := ""
			if t, ok := m["dateTimeCreate"].(time.Time); ok {
				ts = t.UTC().Format(time.RFC3339)
			} else if dt, ok := m["dateTimeCreate"].(primitive.DateTime); ok {
				ts = dt.Time().UTC().Format(time.RFC3339)
			}

			lat := fmt.Sprint(m["latitude"])
			if lat == "<nil>" {
				lat = ""
			}
			lng := fmt.Sprint(m["longitude"])
			if lng == "<nil>" {
				lng = ""
			}

			resp.Details = append(resp.Details, aimodel.PeopleCountingItem{
				ID:          fmt.Sprint(m["id"]),
				SN:          fmt.Sprint(m["sn"]),
				ChannelID:   toInt64(m["channelId"]),
				DateTime:    toTimeISO(m["dateTimeCreate"]),
				Direction:   fmt.Sprint(m["direction"]),
				RegionNames: toStringSlice(m["regionNames"]),
				Timestamp:   ts,
				ImageUrl:    img,

				Lat: lat,
				Lng: lng,

				RegionRois:            toAnySlice(m["regionRois"]),
				PictureCoordinates:    toAnySlice(m["pictureCoordinates"]),
				Zone:                  fmt.Sprint(m["zone"]),
				CameraName:            fmt.Sprint(m["address"]),
				Type:                  fmt.Sprint(m["type"]),
				EventAttributeDetails: m["eventAttributeDetails"],
			})
		}
	}

	return resp, nil
}
