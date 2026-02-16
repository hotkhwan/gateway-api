// internal/services/kschsvc/chatProject.go
package kschsvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kschmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var projectColl = "ksearch_projects"

// internal/services/kschsvc/chatProject.go
func ProjectCreate(ctx context.Context, userId string, req kschmod.Project) (*kschmod.Project, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kschsvc", "ProjectCreate", "kschsvc", "ProjectCreate")
	defer end()

	now := time.Now().UTC()
	req.ID = primitive.NewObjectID()
	req.UserID = userId
	req.CreatedAt = now

	if _, err := stomongo.InsertOne(ctx, projectColl, req); err != nil {
		log.Error().Err(err).Msg("❌ failed to insert project")
		return nil, err
	}

	log.Info().Str("projectId", req.ID.Hex()).Str("userId", userId).Msg("✅ Project created")
	return &req, nil
}

// internal/services/kschsvc/chatProject.go

// List Projects (with pagination)
func ProjectList(ctx context.Context, userId string, page, perPage int, sortField, sortOrder string) ([]kschmod.Project, gmod.Pagination, error) {
	filter := bson.M{"userId": userId}
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/kschsvc",
		"ProjectList",
		"kschsvc",
		"ProjectList",
	)
	defer end()

	opts := options.Find().
		SetSkip(int64((page - 1) * perPage)).
		SetLimit(int64(perPage)).
		SetSort(bson.D{{Key: sortField, Value: sortOrderMap(sortOrder)}})

	var projs []kschmod.Project
	if err := stomongo.Find(ctx, projectColl, filter, opts, &projs); err != nil {
		log.Error().Err(err).Msg("❌ failed to list projects")
		return nil, gmod.Pagination{}, err
	}

	total64, _ := stomongo.Count(ctx, projectColl, filter)
	total := int(total64)

	pagination := gmod.Pagination{
		Page:         page,
		PerPages:     perPage,
		TotalRecords: total,
		TotalPages:   (total + perPage - 1) / perPage,
		SortField:    sortField,
		SortOrder:    sortOrder,
	}

	log.Info().
		Int("count", len(projs)).
		Int("total", total).
		Msg("✅ ProjectList finished")

	return projs, pagination, nil
}

func ProjectGetByID(ctx context.Context, projectId string, page, perPage int) (*kschmod.Project, []kschmod.Chat, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kschsvc", "ProjectGetByID", "kschsvc", "ProjectGetByID")
	defer end()

	oid, err := primitive.ObjectIDFromHex(projectId)
	if err != nil {
		return nil, nil, gmod.Pagination{}, fmt.Errorf("invalid id format")
	}

	var proj kschmod.Project
	if err := stomongo.FindOne(ctx, projectColl, bson.M{"_id": oid}, &proj); err != nil {
		log.Error().Err(err).Str("projectId", projectId).Msg("❌ failed to find project")
		return nil, nil, gmod.Pagination{}, err
	}

	var chats []kschmod.Chat
	filter := bson.M{"projectId": oid}
	opts := options.Find().
		SetSkip(int64((page - 1) * perPage)).
		SetLimit(int64(perPage)).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	if err := stomongo.Find(ctx, chatColl, filter, opts, &chats); err != nil {
		return &proj, nil, gmod.Pagination{}, err
	}

	total64, _ := stomongo.Count(ctx, chatColl, filter)
	pagination := gmod.Pagination{
		Page:         page,
		PerPages:     perPage,
		TotalRecords: int(total64),
		TotalPages:   int((total64 + int64(perPage) - 1) / int64(perPage)),
		SortField:    "createdAt",
		SortOrder:    "desc",
	}

	return &proj, chats, pagination, nil
}

func ProjectUpdate(ctx context.Context, id string, update kschmod.Project) (*kschmod.Project, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kschsvc", "ProjectUpdate", "kschsvc", "ProjectUpdate")
	defer end()

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id format")
	}

	updateData := bson.M{}
	if update.Name != "" {
		updateData["name"] = update.Name
	}

	if len(updateData) == 0 {
		return nil, errors.New("no fields to update")
	}

	// ✅ ไม่ต้อง wrap $set เอง
	if _, err := stomongo.UpdateOne(ctx, projectColl, bson.M{"_id": oid}, updateData); err != nil {
		log.Error().Err(err).Str("projectId", id).Msg("❌ update failed")
		return nil, err
	}

	var proj kschmod.Project
	if err := stomongo.FindOne(ctx, projectColl, bson.M{"_id": oid}, &proj); err != nil {
		return nil, err
	}

	log.Info().Str("projectId", id).Msg("✅ Project updated")
	return &proj, nil
}

func ProjectDelete(ctx context.Context, id string) error {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kschsvc", "ProjectDelete", "kschsvc", "ProjectDelete")
	defer end()

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id format")
	}

	if _, err := stomongo.DeleteOne(ctx, projectColl, bson.M{"_id": oid}); err != nil {
		return err
	}

	if _, err := stomongo.DeleteMany(ctx, chatColl, bson.M{"projectId": oid}); err != nil {
		return err
	}

	log.Info().Str("projectId", id).Msg("✅ Project deleted with chats")
	return nil
}

func sortOrderMap(order string) int {
	if strings.ToLower(order) == "asc" {
		return 1
	}
	return -1
}
