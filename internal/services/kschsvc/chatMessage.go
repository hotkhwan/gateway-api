// internal/services/kschsvc/chatMessage.go
package kschsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/stomongo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kschmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var messageColl = "ksearch_messages"

// ---- Core ChatMessage ----
func ChatMessage(parent context.Context, req kschmod.LLMRequest) (<-chan kschmod.ChatMessage, error) {
	out := make(chan kschmod.ChatMessage)

	go func() {
		defer close(out)

		_, span, log := traceutil.Start(
			parent,
			"github.com/hotkhwan/gateway-api/kschsvc",
			"search.ChatMessage",
			"kschsvc",
			"ChatMessage",
		)
		defer span.End()

		start := time.Now()
		llmResp, err := callMyLLM(parent, req)
		if err != nil {
			log.Error().Err(err).Msg("💥 callMyLLM failed")
			return
		}
		log.Info().
			Int("results", len(llmResp.Details)).
			Dur("took", time.Since(start)).
			Msg("✅ LLM responded")

		for _, det := range llmResp.Details {
			// รวมเป็น prompt สำหรับ summary
			prompt := fmt.Sprintf("Object class: %s, Score: %.2f, Caption: %s",
				det.Class, det.Score, det.Caption)

			var summary string
			if os.Getenv("OPENAI_API_KEY") != "" {
				summary, _ = callChatGPT(parent, prompt)
			} else {
				summary = det.Caption
			}

			msg := kschmod.ChatMessage{
				ID:    primitive.NewObjectID(),
				Img:   det.PathImg,
				Class: det.Class,
				Score: det.Score,
				Meta: map[string]int{
					"x1": int(det.X1),
					"y1": int(det.Y1),
					"x2": int(det.X2),
					"y2": int(det.Y2),
				},
				Caption: det.Caption,
				Summary: summary,
			}

			log.Debug().
				Str("id", msg.ID.Hex()).
				Float64("score", det.Score).
				Msg("➡️ stream message")

			out <- msg
		}
	}()

	return out, nil
}

// ---- Message APIs ----
// internal/services/kschsvc/chatMessage.go
func MessageCreateOrStream(ctx context.Context, userId, chatId, prompt string) <-chan kschmod.ChatMessage {
	out := make(chan kschmod.ChatMessage)

	go func() {
		defer close(out)

		ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kschsvc", "MessageCreateOrStream", "kschsvc", "MessageCreateOrStream")
		defer end()

		var oid primitive.ObjectID
		var chatName string
		if chatId == "" {
			// ❌ ไม่มี chatId → ต้องสร้าง chat ใหม่
			runes := []rune(prompt)
			if len(runes) > 20 {
				chatName = string(runes[:20])
			} else {
				chatName = prompt
			}
			newChat := kschmod.Chat{
				ID:        primitive.NewObjectID(),
				UserID:    userId, // ✅ ใส่ user จาก context
				Name:      chatName,
				CreatedAt: time.Now().UTC(),
			}
			// _, _ = stomongo.InsertOne(ctx, chatColl, newChat)

			dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if id, err := stomongo.InsertOne(dbCtx, chatColl, newChat); err != nil {
				log.Error().Err(err).Interface("chat", newChat).Msg("❌ failed to insert chat")
				return
			} else {
				log.Info().Str("chatId", id.Hex()).Interface("chat", newChat).Msg("✅ inserted chat")
			}

			oid = newChat.ID
			log.Info().Str("chatId", oid.Hex()).Str("userId", userId).Msg("✅ created new chat for message")
		} else {
			// มี chatId → ใช้งานต่อ
			var err error
			oid, err = primitive.ObjectIDFromHex(chatId)
			if err != nil {
				log.Error().Str("chatId", chatId).Msg("❌ invalid chatId format")
				return
			}
		}

		// 1) insert user message
		userMsg := kschmod.ChatMessage{
			ID:        primitive.NewObjectID(),
			ChatID:    oid,
			Role:      "user",
			Prompt:    prompt,
			CreatedAt: time.Now().UTC(),
		}
		msgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if id, err := stomongo.InsertOne(msgCtx, messageColl, userMsg); err != nil {
			log.Error().Err(err).Interface("msg", userMsg).Msg("❌ failed to insert user message")
		} else {
			log.Info().Str("msgId", id.Hex()).Msg("✅ inserted user message")
		}
		out <- userMsg

		// userMsg := kschmod.ChatMessage{
		// 	ID:        primitive.NewObjectID(),
		// 	ChatID:    oid,
		// 	Role:      "user",
		// 	Prompt:    prompt,
		// 	CreatedAt: time.Now().UTC(),
		// }
		// _, _ = stomongo.InsertOne(ctx, messageColl, userMsg)
		// out <- userMsg

		// 2) call LLM + stream assistant messages
		llmChan, _ := ChatMessage(ctx, kschmod.LLMRequest{
			Prompt:  prompt,
			NameID:  "vector_ai",
			Page:    intPtr(1),
			PerPage: intPtr(5),
		})
		for resp := range llmChan {
			msg := kschmod.ChatMessage{
				ID:        primitive.NewObjectID(),
				ChatID:    oid,
				Role:      "assistant",
				Img:       resp.Img,
				Class:     resp.Class,
				Score:     resp.Score,
				Meta:      resp.Meta,
				Caption:   resp.Caption,
				Summary:   resp.Summary,
				CreatedAt: time.Now().UTC(),
			}
			msgCtx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)

			if id, err := stomongo.InsertOne(msgCtx2, messageColl, msg); err != nil {
				log.Error().Err(err).Interface("msg", msg).Msg("❌ failed to insert assistant message")
			} else {
				log.Info().Str("msgId", id.Hex()).Msg("✅ inserted assistant message")
			}

			cancel2() // ✅ ปล่อย resource ทันที
			out <- msg
		}
	}()

	return out
}

// MessageCreateStream: บันทึกข้อความ user → call LLM → บันทึก assistant ตอบ
func MessageCreateStream(ctx context.Context, chatId, prompt string) <-chan kschmod.ChatMessage {
	out := make(chan kschmod.ChatMessage)

	go func() {
		defer close(out)

		ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kschsvc", "MessageCreateStream", "kschsvc", "MessageCreateStream")
		defer end()

		// แปลง chatId → ObjectID
		oid, err := primitive.ObjectIDFromHex(chatId)
		if err != nil {
			log.Error().Str("chatId", chatId).Msg("❌ invalid chatId format")
			return
		}

		// 1) Insert user message
		userMsg := kschmod.ChatMessage{
			ID:        primitive.NewObjectID(),
			ChatID:    oid,
			Role:      "user",
			Prompt:    prompt,
			CreatedAt: time.Now().UTC(),
		}
		_, _ = stomongo.InsertOne(ctx, messageColl, userMsg)
		out <- userMsg

		// 2) Call LLM + stream responses
		llmChan, _ := ChatMessage(ctx, kschmod.LLMRequest{
			Prompt:  prompt,
			NameID:  "vector_ai",
			Page:    intPtr(1),
			PerPage: intPtr(5),
		})

		for resp := range llmChan {
			msg := kschmod.ChatMessage{
				ID:        primitive.NewObjectID(),
				ChatID:    oid,
				Role:      "assistant",
				Img:       resp.Img,
				Class:     resp.Class,
				Score:     resp.Score,
				Meta:      resp.Meta,
				Caption:   resp.Caption,
				Summary:   resp.Summary,
				CreatedAt: time.Now().UTC(),
			}
			_, _ = stomongo.InsertOne(ctx, messageColl, msg)
			out <- msg
		}
	}()

	return out
}

// MessageList: ดึง message ทั้งหมดของ chatId + pagination
func MessageList(
	ctx context.Context,
	chatId string,
	page, perPage int,
	filters map[string]string,
	sortField, sortOrder string,
) ([]kschmod.ChatMessage, gmod.Pagination, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kschsvc", "MessageList", "kschsvc", "MessageList")
	defer end()

	oid, err := primitive.ObjectIDFromHex(chatId)
	if err != nil {
		return nil, gmod.Pagination{}, errors.New("invalid chatId format")
	}

	// filter base
	query := bson.M{"chatId": oid}
	if v := filters["state"]; v != "" {
		query["state"] = v
	}
	if v := filters["status"]; v != "" {
		query["status"] = v
	}
	if v := filters["search"]; v != "" {
		// full-text search หรือ regex
		query["prompt"] = bson.M{"$regex": v, "$options": "i"}
	}

	// sort
	sort := bson.D{{Key: sortField, Value: -1}}
	if sortOrder == "asc" {
		sort = bson.D{{Key: sortField, Value: 1}}
	}

	// ใช้ stomongo.FindWithPagination
	var msgs []kschmod.ChatMessage
	pagination, err := stomongo.FindWithPagination(ctx, messageColl, query, sort, page, perPage, &msgs)
	if err != nil {
		log.Error().Err(err).Str("chatId", chatId).Msg("❌ failed to list messages with pagination")
		return nil, pagination, err
	}

	// เติมค่า sortField + sortOrder ลงไปใน pagination struct
	pagination.SortField = sortField
	pagination.SortOrder = sortOrder

	return msgs, pagination, nil
}

// ---- LLM / GPT Helpers ----
func callMyLLM(parent context.Context, req kschmod.LLMRequest) (*kschmod.LLMResponse, error) {
	_, span, log := traceutil.Start(parent,
		"github.com/hotkhwan/gateway-api/kschsvc",
		"search.callMyLLM",
		"kschsvc",
		"callMyLLM",
	)
	defer span.End()

	// timeout 30s
	reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// pointer check
	page := 1
	if req.Page != nil {
		page = *req.Page
	}
	perPage := 10
	if req.PerPage != nil {
		perPage = *req.PerPage
	}

	cfg := config.GetLLMConfig()
	url := fmt.Sprintf("%s/camSearch/api/search?nameID=%s&page=%d&perPages=%d&sortOrder=%s&sortField=%s",
		cfg.MyLLMUrl,
		req.NameID,
		page,
		perPage,
		ifEmpty(req.SortOrder, "desc"),
		ifEmpty(req.SortField, "id"),
	)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if req.Prompt != "" {
		_ = writer.WriteField("text", req.Prompt)
	} else if req.ImagePath != "" {
		file, err := os.Open(req.ImagePath)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		part, err := writer.CreateFormFile("image", filepath.Base(req.ImagePath))
		if err != nil {
			return nil, err
		}
		_, _ = io.Copy(part, file)
	}
	writer.Close()

	httpReq, _ := http.NewRequestWithContext(reqCtx, "POST", url, body)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Msg("❌ LLM request failed")
		return nil, err
	}
	defer resp.Body.Close()

	var result kschmod.LLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func callChatGPT(parent context.Context, text string) (string, error) {
	_, span, log := traceutil.Start(parent,
		"github.com/hotkhwan/gateway-api/kschsvc",
		"search.callChatGPT",
		"kschsvc",
		"callChatGPT",
	)
	defer span.End()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", nil
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	payload := strings.NewReader(fmt.Sprintf(`{
        "model": "gpt-4o-mini",
        "messages": [{"role":"user","content":"สรุปให้อ่านง่าย: %s"}],
        "max_tokens": 200
    }`, text))

	req, _ := http.NewRequestWithContext(reqCtx,
		"POST", "https://api.openai.com/v1/chat/completions", payload)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("❌ chatgpt http failed")
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return string(raw), nil
	}

	if len(parsed.Choices) > 0 {
		return parsed.Choices[0].Message.Content, nil
	}
	return "", nil
}

func MessageUpdate(ctx context.Context, chatId string, messageId string, update kschmod.MessageUpdateRequest) (*kschmod.Message, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/kschsvc", "MessageUpdate", "kschsvc", "MessageUpdate")
	defer end()

	chatOid, err := primitive.ObjectIDFromHex(chatId)
	if err != nil {
		return nil, errors.New("invalid chatId format")
	}
	msgOid, err := primitive.ObjectIDFromHex(messageId)
	if err != nil {
		return nil, errors.New("invalid messageId format")
	}

	updateData := bson.M{}
	if update.Prompt != "" {
		updateData["prompt"] = update.Prompt
	}
	if update.Caption != "" {
		updateData["caption"] = update.Caption
	}
	if update.Img != "" {
		updateData["img"] = update.Img
	}
	if update.Class != "" {
		updateData["class"] = update.Class
	}

	if len(updateData) == 0 {
		return nil, errors.New("no fields to update")
	}

	if _, err := stomongo.UpdateOne(ctx, messageColl, bson.M{
		"_id":    msgOid,
		"chatId": chatOid,
	}, bson.M{"$set": updateData}); err != nil {
		log.Error().Err(err).Str("chatId", chatId).Str("messageId", messageId).Msg("❌ update failed")
		return nil, err
	}

	var msg kschmod.Message
	if err := stomongo.FindOne(ctx, messageColl, bson.M{"_id": msgOid}, &msg); err != nil {
		return nil, err
	}

	log.Info().Str("chatId", chatId).Str("messageId", messageId).Msg("✅ Message updated")
	return &msg, nil
}

// ---- Helpers ----
func ifEmpty(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func intPtr(v int) *int {
	return &v
}
