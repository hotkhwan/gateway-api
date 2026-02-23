// internal/services/devsvc/device.go
package devsvc

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/internal/services/memsvc"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/devmod"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/utils"
	"github.com/hotkhwan/gateway-api/utils/authutil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DevicesList คืนค่า: รายการ, pagination, online, offline, error
func DevicesList(ctx context.Context, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]devmod.Device, gmod.Pagination, int, int, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/devsvc",
		"devices.DevicesList",
		"devsvc", "DevicesList",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log.Debug().
		Int("page", page).
		Int("perPages", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Interface("filters", filters).
		Msg("📥 Listing devices")

	// 1) Base Mongo filter
	baseFilter := bson.M{"isDeleted": bson.M{"$ne": true}}

	if id := filters["id"]; id != "" {
		if objID, err := primitive.ObjectIDFromHex(id); err == nil {
			baseFilter["_id"] = objID
		} else {
			baseFilter["_id"] = id
			log.Warn().Err(err).Str("id", id).Msg("⚠️ Invalid ObjectID in filter, fallback to string id")
		}
	}
	if search := filters["search"]; search != "" {
		baseFilter["name"] = bson.M{"$regex": search, "$options": "i"}
	}

	if page < 1 {
		page = 1
	}
	if perPages <= 0 {
		perPages = 10
	}

	skip := (page - 1) * perPages
	sortVal := -1
	if sortOrder == "asc" {
		sortVal = 1
	}
	if sortField == "" {
		sortField = "dateTimeCreate"
	}

	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(perPages)).
		SetSort(bson.D{{Key: sortField, Value: sortVal}}).
		SetProjection(bson.M{"isDeleted": 0})

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("camera")

	// 2) Query Mongo
	var rawResults []devmod.DeviceMongo
	cursor, err := coll.Find(ctx, baseFilter, opts)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to find devices")
		return nil, gmod.Pagination{}, 0, 0, err
	}
	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &rawResults); err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode device results")
		return nil, gmod.Pagination{}, 0, 0, err
	}

	total, err := coll.CountDocuments(ctx, baseFilter)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to count documents")
		return nil, gmod.Pagination{}, 0, 0, err
	}

	log.Debug().
		Int("count", len(rawResults)).
		Int64("total", total).
		Msg("✅ Devices listed successfully (before permission filter)")

	// 3) Map Mongo → DTO
	devices := make([]devmod.Device, 0, len(rawResults))
	for _, d := range rawResults {
		devices = append(devices, devmod.Device{
			ID:          d.ID.Hex(),
			Name:        d.Name,
			User:        d.User,
			Password:    d.Password,
			URL:         d.URL,
			District:    d.District,
			Lat:         d.Lat,
			Lng:         d.Lng,
			AtaWsFlvUrl: d.AtaWsFlvUrl,
			Brand:       d.Brand,
			Status:      d.Status,
			State:       d.State,
			Roi:         d.Roi,
			CreateAt:    utils.FormatTimeOrEmpty(d.CreateAt),
			UpdateAt:    utils.FormatTimeOrEmpty(d.UpdateAt),
		})
	}

	//Check GroupMembership

	pagination := gmod.Pagination{
		Page:         page,
		PerPages:     perPages,
		TotalRecords: int(total),
		TotalPages:   (int(total) + perPages - 1) / perPages,
		SortField:    sortField,
		SortOrder:    sortOrder,
	}

	// 4) Online / Offline (ยังเป็น global count)
	onlineFilter := bson.M{}
	for k, v := range baseFilter {
		onlineFilter[k] = v
	}
	onlineFilter["status"] = true

	offlineFilter := bson.M{}
	for k, v := range baseFilter {
		offlineFilter[k] = v
	}
	offlineFilter["$or"] = []bson.M{
		{"status": false},
		{"status": bson.M{"$exists": false}},
	}

	onlineCount, err := coll.CountDocuments(ctx, onlineFilter)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to count online")
		return devices, pagination, 0, 0, err
	}

	offlineCount, err := coll.CountDocuments(ctx, offlineFilter)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to count offline")
		return devices, pagination, int(onlineCount), 0, err
	}

	log.Debug().
		Int64("online", onlineCount).
		Int64("offline", offlineCount).
		Msg("📊 Online/Offline summarized (before permission filter)")

	// 5) 🔐 Permission filter (Permify)
	subjectType := filters["subjectType"] // เช่น "role"
	subjectId := filters["subjectId"]     // เช่น "user" หรือ "mgt"

	if subjectType != "" && subjectId != "" && len(devices) > 0 {
		log.Debug().
			Str("subjectType", subjectType).
			Str("subjectId", subjectId).
			Int("deviceCount", len(devices)).
			Msg("🔐 Filtering devices by permission (Permify batch)")

		inputs := make([]authzmod.PermissionCheckInput, 0, len(devices))
		for _, d := range devices {
			resourceID := "cam_" + d.ID // ต้องตรงกับ tuple ใน Permify

			inputs = append(inputs, authzmod.PermissionCheckInput{
				EntityType:  "resource",
				EntityID:    resourceID,
				Permission:  "view",
				SubjectType: subjectType,
				SubjectID:   subjectId,
			})
		}

		log.Debug().
			Interface("perm_inputs", inputs).
			Msg("🔐 [DevicesList] Permission check inputs")

		allowedMap, err := authzsvc.CheckPermissionsBatch(ctx, inputs)
		if err != nil {
			log.Error().Err(err).Msg("❌ CheckPermissionsBatch failed")
			return nil, gmod.Pagination{}, 0, 0, err
		}

		log.Debug().
			Interface("perm_allowed_map", allowedMap).
			Msg("🔐 [DevicesList] Permission allowed map")

		filtered := make([]devmod.Device, 0, len(devices))
		for _, d := range devices {
			resourceID := "cam_" + d.ID
			if allowed, ok := allowedMap[resourceID]; ok && allowed {
				filtered = append(filtered, d)
			}
		}

		log.Debug().
			Int("before", len(devices)).
			Int("after", len(filtered)).
			Msg("✅ Devices filtered by permission")

		// devices = filtered
		devices = devices
	}

	return devices, pagination, int(onlineCount), int(offlineCount), nil
}

func DevicesListCheckPermission(ctx context.Context, token string, page, perPages int, filters map[string]string, sortField, sortOrder string) ([]devmod.Device, gmod.Pagination, int, int, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/devsvc",
		"devices.DevicesList",
		"devsvc", "DevicesList",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	claims, err := authutil.ValidateJWT(token)
	userId := claims["sub"].(string)
	//"7c681de1-ac3d-4a35-821b-2cd09cf917c2"
	members, err := memsvc.MembersGetByUserID(ctx, userId)
	if err != nil {
		log.Warn().Err(err).Str("UserId", userId).Msg(err.Error())
	}
	groups := []string{}
	for _, m := range members {
		isRsult, err := authzsvc.PermissionCheck(
			ctx,
			50,
			"resource",
			"cameraGroup:"+m.GroupID,
			"view",
			"group",
			m.GroupID,
		)
		if err != nil {
			log.Error().Err(err).Msg("❌ Failed to check permission")
			return nil, gmod.Pagination{}, 0, 0, err
		}

		if isRsult {
			groups = append(groups, m.GroupID)
		}
	}

	log.Debug().
		Int("page", page).
		Int("perPages", perPages).
		Str("sortField", sortField).
		Str("sortOrder", sortOrder).
		Interface("filters", filters).
		Msg("📥 Listing devices")

	// 1) Base Mongo filter
	baseFilter := bson.M{"isDeleted": bson.M{"$ne": true}}

	if id := filters["id"]; id != "" {
		if objID, err := primitive.ObjectIDFromHex(id); err == nil {
			baseFilter["_id"] = objID
		} else {
			baseFilter["_id"] = id
			log.Warn().Err(err).Str("id", id).Msg("⚠️ Invalid ObjectID in filter, fallback to string id")
		}
	}
	if search := filters["search"]; search != "" {
		baseFilter["name"] = bson.M{"$regex": search, "$options": "i"}
	}
	if len(groups) > 0 {
		baseFilter["groupIds"] = bson.M{"$in": groups}
	}

	if page < 1 {
		page = 1
	}
	if perPages <= 0 {
		perPages = 10
	}

	skip := (page - 1) * perPages
	sortVal := -1
	if sortOrder == "asc" {
		sortVal = 1
	}
	if sortField == "" {
		sortField = "dateTimeCreate"
	}

	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(perPages)).
		SetSort(bson.D{{Key: sortField, Value: sortVal}}).
		SetProjection(bson.M{"isDeleted": 0})

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("camera")

	// 2) Query Mongo  filter by groups
	var rawResults []devmod.DeviceMongo
	cursor, err := coll.Find(ctx, baseFilter, opts)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to find devices")
		return nil, gmod.Pagination{}, 0, 0, err
	}
	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &rawResults); err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode device results")
		return nil, gmod.Pagination{}, 0, 0, err
	}

	total, err := coll.CountDocuments(ctx, baseFilter)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to count documents")
		return nil, gmod.Pagination{}, 0, 0, err
	}

	log.Debug().
		Int("count", len(rawResults)).
		Int64("total", total).
		Msg("✅ Devices listed successfully (before permission filter)")

	// 3) Map Mongo → DTO
	devices := make([]devmod.Device, 0, len(rawResults))
	for _, d := range rawResults {
		if d.Public {
			devices = append(devices, devmod.Device{
				ID:          d.ID.Hex(),
				Name:        d.Name,
				User:        d.User,
				Password:    d.Password,
				URL:         d.URL,
				District:    d.District,
				Lat:         d.Lat,
				Lng:         d.Lng,
				AtaWsFlvUrl: d.AtaWsFlvUrl,
				Brand:       d.Brand,
				Status:      d.Status,
				State:       d.State,
				Roi:         d.Roi,
				Public:      d.Public,
				CreateAt:    utils.FormatTimeOrEmpty(d.CreateAt),
				UpdateAt:    utils.FormatTimeOrEmpty(d.UpdateAt),
			})
		}
	}
	// region Get devicePublic Data
	{
		publicFilter := bson.M{
			"isDeleted":   bson.M{"$ne": true},
			"groupIds":    bson.M{"$size": 0}, // ✅ ไม่มี group
			"Access_type": "public",           // ✅ ต้องเป็น public
		}

		cursorPub, err := coll.Find(ctx, publicFilter, opts)
		if err != nil {
			log.Error().Err(err).Msg("❌ Failed to find public devices")
			return nil, gmod.Pagination{}, 0, 0, err
		}
		defer cursorPub.Close(ctx)

		var publicResults []devmod.DeviceMongo
		if err := cursorPub.All(ctx, &publicResults); err != nil {
			log.Error().Err(err).Msg("❌ Failed to decode public device results")
			return nil, gmod.Pagination{}, 0, 0, err
		}

		for _, d := range publicResults {
			devices = append(devices, devmod.Device{
				ID:          d.ID.Hex(),
				Name:        d.Name,
				User:        d.User,
				Password:    d.Password,
				URL:         d.URL,
				District:    d.District,
				Lat:         d.Lat,
				Lng:         d.Lng,
				AtaWsFlvUrl: d.AtaWsFlvUrl,
				Brand:       d.Brand,
				Status:      d.Status,
				State:       d.State,
				Roi:         d.Roi,
				Public:      d.Public,
				CreateAt:    utils.FormatTimeOrEmpty(d.CreateAt),
				UpdateAt:    utils.FormatTimeOrEmpty(d.UpdateAt),
			})
		}
	}
	// endregion

	//Check GroupMembership
	pagination := gmod.Pagination{
		Page:         page,
		PerPages:     perPages,
		TotalRecords: int(total),
		TotalPages:   (int(total) + perPages - 1) / perPages,
		SortField:    sortField,
		SortOrder:    sortOrder,
	}

	// 4) Online / Offline (ยังเป็น global count)
	onlineFilter := bson.M{}
	for k, v := range baseFilter {
		onlineFilter[k] = v
	}
	onlineFilter["status"] = true

	offlineFilter := bson.M{}
	for k, v := range baseFilter {
		offlineFilter[k] = v
	}
	offlineFilter["$or"] = []bson.M{
		{"status": false},
		{"status": bson.M{"$exists": false}},
	}

	onlineCount, err := coll.CountDocuments(ctx, onlineFilter)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to count online")
		return devices, pagination, 0, 0, err
	}

	offlineCount, err := coll.CountDocuments(ctx, offlineFilter)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to count offline")
		return devices, pagination, int(onlineCount), 0, err
	}

	log.Debug().
		Int64("online", onlineCount).
		Int64("offline", offlineCount).
		Msg("📊 Online/Offline summarized (before permission filter)")

	// 5) 🔐 Permission filter (Permify)
	subjectType := filters["subjectType"] // เช่น "role"
	subjectId := filters["subjectId"]     // เช่น "user" หรือ "mgt"

	if subjectType != "" && subjectId != "" && len(devices) > 0 {
		log.Debug().
			Str("subjectType", subjectType).
			Str("subjectId", subjectId).
			Int("deviceCount", len(devices)).
			Msg("🔐 Filtering devices by permission (Permify batch)")

		inputs := make([]authzmod.PermissionCheckInput, 0, len(devices))
		for _, d := range devices {
			resourceID := "cam_" + d.ID // ต้องตรงกับ tuple ใน Permify

			inputs = append(inputs, authzmod.PermissionCheckInput{
				EntityType:  "resource",
				EntityID:    resourceID,
				Permission:  "view",
				SubjectType: subjectType,
				SubjectID:   subjectId,
			})
		}

		log.Debug().
			Interface("perm_inputs", inputs).
			Msg("🔐 [DevicesList] Permission check inputs")

		allowedMap, err := authzsvc.CheckPermissionsBatch(ctx, inputs)
		if err != nil {
			log.Error().Err(err).Msg("❌ CheckPermissionsBatch failed")
			return nil, gmod.Pagination{}, 0, 0, err
		}

		log.Debug().
			Interface("perm_allowed_map", allowedMap).
			Msg("🔐 [DevicesList] Permission allowed map")

		filtered := make([]devmod.Device, 0, len(devices))
		for _, d := range devices {
			resourceID := "cam_" + d.ID
			if allowed, ok := allowedMap[resourceID]; ok && allowed {
				filtered = append(filtered, d)
			}
		}

		log.Debug().
			Int("before", len(devices)).
			Int("after", len(filtered)).
			Msg("✅ Devices filtered by permission")

		//devices = filtered
		//devices = devices
	}

	return devices, pagination, int(onlineCount), int(offlineCount), nil
}

// DeviceGetByID ค้นหา device ตาม id (รองรับทั้ง ObjectID และ string)
func DeviceGetByID(ctx context.Context, id string) (*devmod.Device, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/devsvc",
		"devices.DeviceGetByID",
		"devsvc", "DeviceGetByID",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("camera")

	// ไม่เลือก doc ที่ isDeleted=true
	filter := bson.M{"isDeleted": bson.M{"$ne": true}}
	if oid, err := primitive.ObjectIDFromHex(id); err == nil {
		filter["_id"] = oid
	} else {
		filter["_id"] = id
	}

	opts := options.FindOne().SetProjection(bson.M{"isDeleted": 0})

	log.Debug().Interface("filter", filter).Msg("🔎 Get device by id")

	var raw devmod.DeviceMongo
	if err := coll.FindOne(ctx, filter, opts).Decode(&raw); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			log.Warn().Str("id", id).Msg("⚠️ Device not found or deleted")
			return nil, mongo.ErrNoDocuments
		}
		log.Error().Err(err).Str("id", id).Msg("❌ Failed to get device")
		return nil, err
	}

	dev := &devmod.Device{
		ID:          raw.ID.Hex(),
		Name:        raw.Name,
		User:        raw.User,
		Password:    raw.Password,
		URL:         raw.URL,
		District:    raw.District,
		Lat:         raw.Lat,
		Lng:         raw.Lng,
		AtaWsFlvUrl: raw.AtaWsFlvUrl,
		Brand:       raw.Brand,
		Status:      raw.Status,
		State:       raw.State,
		Roi:         raw.Roi,
		CreateAt:    utils.FormatTimeOrEmpty(raw.CreateAt),
		UpdateAt:    utils.FormatTimeOrEmpty(raw.UpdateAt),
	}

	log.Debug().Str("id", dev.ID).Str("name", dev.Name).Msg("✅ DeviceGetByID success")
	return dev, nil
}

// DeviceCreate: คงของเดิม (ใส่ state/status ตอนสร้าง)
func DeviceCreate(ctx context.Context, device devmod.Device) (map[string]interface{}, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/devsvc",
		"devices.DeviceCreate",
		"devsvc", "DeviceCreate",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log.Debug().
		Str("name", device.Name).
		Str("user", device.User).
		Msg("📦 Creating new device")

	deviceMap := bson.M{
		"name":           device.Name,
		"user":           device.User,
		"password":       device.Password,
		"url":            device.URL,
		"district":       device.District,
		"lat":            device.Lat,
		"lng":            device.Lng,
		"ataWsFlvUrl":    device.AtaWsFlvUrl,
		"brand":          device.Brand,
		"status":         true,
		"state":          "create",
		"dateTimeCreate": time.Now(),
	}
	if len(device.Roi) > 0 {
		deviceMap["roi"] = device.Roi
	}
	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("camera")

	res, err := coll.InsertOne(ctx, deviceMap)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to insert device")
		return nil, err
	}

	log.Debug().Interface("inserted_id", res.InsertedID).Msg("✅ Device created successfully")

	return map[string]interface{}{
		"message":     "Create camera successfully",
		"inserted_id": res.InsertedID,
	}, nil
}

// DeviceUpdate: คงของเดิม
func DeviceUpdate(ctx context.Context, id string, data devmod.Device) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/devsvc",
		"devices.DeviceUpdate",
		"devsvc", "DeviceUpdate",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log.Info().Str("id", id).Msg("✏️ Updating device")

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		log.Warn().Err(err).Msg("⚠️ Invalid ObjectID for update")
		return errors.New("invalid ObjectID")
	}

	update := bson.M{
		"$set": bson.M{
			"name":     data.Name,
			"user":     data.User,
			"password": data.Password,
			"url":      data.URL,
			"district": data.District,
			"lat":      data.Lat,
			"lng":      data.Lng,
			// "ataWsFlvUrl":    data.AtaWsFlvUrl,
			// "brand":          data.Brand,
			"roi":            data.Roi,
			"dateTimeUpdate": time.Now(),
			"state":          "update",
		},
	}

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("camera")

	_, err = coll.UpdateByID(ctx, objID, update)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("❌ Failed to update device")
		return err
	}

	log.Info().Str("id", id).Msg("✅ Device updated")
	return nil
}

// DeviceDelete: soft delete
func DeviceDelete(ctx context.Context, id string) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/devsvc",
		"devices.DeviceDelete",
		"devsvc", "DeviceDelete",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("camera")

	// รองรับทั้ง ObjectID และ string id
	filter := bson.M{}
	if oid, err := primitive.ObjectIDFromHex(id); err == nil {
		filter["_id"] = oid
	} else {
		filter["_id"] = id
	}

	update := bson.M{
		"$set": bson.M{
			"isDeleted":      true,
			"dateTimeUpdate": time.Now(),
		},
	}

	log.Info().Str("id", id).Msg("🗑️ Soft-deleting device (isDeleted=true)")

	res, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("❌ Failed to soft delete device")
		return err
	}
	if res.MatchedCount == 0 {
		log.Warn().Str("id", id).Msg("⚠️ Device not found for soft delete")
		return mongo.ErrNoDocuments
	}

	log.Info().Str("id", id).Msg("✅ Device soft-deleted (isDeleted=true)")
	return nil
}
