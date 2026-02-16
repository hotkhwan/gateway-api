// internal/services/kschsvc/chatStory.go
package kschsvc

import (
	"context"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/kschmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var chatColl = "ksearch_chats"

// Create Chat
func ChatCreate(ctx context.Context, req kschmod.ChatRequest, userId string) (*kschmod.Chat, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kschsvc", "ChatCreate", "kschsvc", "ChatCreate")
	defer end()

	var projectOID *primitive.ObjectID
	if req.ProjectID != "" {
		oid, err := primitive.ObjectIDFromHex(req.ProjectID)
		if err != nil {
			return nil, errors.New("invalid projectId format")
		}
		projectOID = &oid
	}

	chat := kschmod.Chat{
		ID:        primitive.NewObjectID(),
		UserID:    userId,
		ProjectID: projectOID, // ✅ อนุญาตให้เป็น nil
		Name:      req.Name,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := stomongo.InsertOne(ctx, chatColl, chat); err != nil {
		log.Error().Err(err).Msg("❌ failed to insert chat")
		return nil, err
	}

	if projectOID != nil {
		log.Debug().
			Str("chatId", chat.ID.Hex()).
			Str("projectId", projectOID.Hex()).
			Msg("✅ Chat created in project")

	} else {
		log.Debug().Str("chatId", chat.ID.Hex()).Msg("✅ Chat created (standalone)")
	}
	return &chat, nil
}

func ChatList(ctx context.Context, projectId, userId string) ([]kschmod.Chat, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kschsvc", "ChatList", "kschsvc", "ChatList")
	defer end()

	filter := bson.M{"userId": userId}
	if projectId != "" {
		oid, err := primitive.ObjectIDFromHex(projectId)
		if err != nil {
			return nil, errors.New("invalid projectId format")
		}
		filter["projectId"] = oid
	}

	var chats []kschmod.Chat
	if err := stomongo.Find(ctx, chatColl, filter, nil, &chats); err != nil {
		log.Error().Err(err).Str("projectId", projectId).Msg("❌ failed to list chats")
		return nil, err
	}
	return chats, nil
}

func ChatUpdate(ctx context.Context, id string, update kschmod.Chat) (*kschmod.Chat, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kschsvc", "ChatUpdate", "kschsvc", "ChatUpdate")
	defer end()

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errors.New("invalid chatId format")
	}

	updateData := bson.M{}
	if update.Name != "" {
		updateData["name"] = update.Name
	}

	if len(updateData) == 0 {
		return nil, errors.New("no fields to update")
	}

	if _, err := stomongo.UpdateOne(ctx, chatColl, bson.M{"_id": oid}, updateData); err != nil {
		log.Error().Err(err).Str("chatId", id).Msg("❌ update failed")
		return nil, err
	}

	var chat kschmod.Chat
	if err := stomongo.FindOne(ctx, chatColl, bson.M{"_id": oid}, &chat); err != nil {
		return nil, err
	}

	log.Info().Str("chatId", id).Msg("✅ Chat updated")
	return &chat, nil
}

func ChatDelete(ctx context.Context, id string) error {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kschsvc", "ChatDelete", "kschsvc", "ChatDelete")
	defer end()

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return errors.New("invalid chatId format")
	}

	// ลบ Chat เอง
	if _, err := stomongo.DeleteOne(ctx, chatColl, bson.M{"_id": oid}); err != nil {
		log.Error().Err(err).Str("chatId", id).Msg("❌ failed to delete chat")
		return err
	}

	// ลบ Messages ทั้งหมดที่อยู่ใน Chat นั้น
	if _, err := stomongo.DeleteMany(ctx, messageColl, bson.M{"chatId": oid}); err != nil {
		log.Error().Err(err).Str("chatId", id).Msg("❌ failed to delete related messages")
		return err
	}

	log.Info().Str("chatId", id).Msg("✅ Chat and related messages deleted")
	return nil
}
