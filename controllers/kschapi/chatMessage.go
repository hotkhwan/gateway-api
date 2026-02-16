// controllers/kschapi/chatMessage.go
package kschapi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/services/kschsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kschmod"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/mongo"
)

// ChatMessage godoc
// @Summary Search with LLM + summarize
// @Description Call internal LLM and ChatGPT, stream results
// @Tags Ksearch/Chats
// @Accept json
// @Produce text/event-stream
// @Param body body kschmod.LLMRequest true "Prompt request"
// @Success 200 {string} string "event-stream"
// @Router /ksearch/chat [post]
// @Security BearerAuth
func ChatMessage(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(
		c.UserContext(),
		"github.com/hotkhwan/gateway-api/kschapi",
		"search.ChatMessage",
		"kschapi",
		"ChatMessage",
	)
	defer span.End()

	var req kschmod.LLMRequest
	if err := c.BodyParser(&req); err != nil {
		log.Error().Err(err).Msg("❌ invalid request body")
		return httputil.FailBadRequest(c, "BAD_REQUEST", "invalid body")
	}

	log.Info().
		Str("prompt", req.Prompt).
		Str("nameId", req.NameID).
		Msg("📩 incoming chat request")

	ch, err := kschsvc.ChatMessage(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("💥 ChatMessage failed to start")
		return c.Status(500).JSON(fiber.Map{"error": "ChatMessage start failed"})
	}

	// SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	// ใช้ bufio.Writer สำหรับ stream
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		for msg := range ch {
			data, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.Flush()
		}
		fmt.Fprint(w, "event: end\ndata: done\n\n")
		w.Flush()
	})

	log.Info().Msg("✅ ChatMessage finished")
	return nil
}

// MessageCreateStream godoc
// @Summary Create new message in a chat and stream assistant replies
// @Tags Ksearch/Chats
// @Accept json
// @Produce text/event-stream
// @Param chatId path string false "Chat ID"
// @Param body body object{prompt=string} true "Prompt request"
// @Success 200 {string} string "event-stream"
// @Router /ksearch/chats/messages [post]
// @Router /ksearch/chats/{chatId}/messages [post]
// @Security BearerAuth
func MessageCreateStream(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(
		c.UserContext(),
		"github.com/hotkhwan/gateway-api/kschapi",
		"MessageCreateStream",
		"kschapi",
		"MessageCreateStream",
	)
	defer span.End()

	userId, err := middleware.CurrentUserID(c)
	if err != nil {
		return httputil.FailUnauthorized(c, "UNAUTHORIZED", err.Error())
	}

	chatId := c.Params("chatId") // อาจจะว่างถ้าเรียก /chats/messages
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := c.BodyParser(&body); err != nil || body.Prompt == "" {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	// SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	// เรียก service
	ch := kschsvc.MessageCreateOrStream(ctx, userId, chatId, body.Prompt)

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		for msg := range ch {
			data, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.Flush()
		}
		fmt.Fprint(w, "event: end\ndata: done\n\n")
		w.Flush()
	})
	log.Info().Str("chatId", chatId).Msg("✅ MessageCreateStream finished")
	return nil
}

// MessageList godoc
// @Summary List messages in a chat with pagination
// @Tags Ksearch/Chats
// @Accept json
// @Produce json
// @Param chatId path string true "Chat ID"
// @Param page query int false "Page number" default(1)
// @Param perPage query int false "Items per page" default(10)
// @Param sortField query string false "Sort field" default(createdAt)
// @Param sortOrder query string false "Sort order (asc/desc)" default(desc)
// @Param search query string false "Search text"
// @Param state query string false "Filter by state"
// @Param status query string false "Filter by status"
// @Success 200 {object} kschmod.ChatMessagePaginationResponse
// @Router /ksearch/chats/{chatId}/messages [get]
// @Security BearerAuth
func MessageList(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "github.com/hotkhwan/gateway-api/kschapi", "message.List", "kschapi", "MessageList")
	defer span.End()

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("perPage", "10"))
	sortField := c.Query("sortField", "createdAt")
	sortOrder := c.Query("sortOrder", "asc")

	filters := map[string]string{
		"search": c.Query("search"),
		"state":  c.Query("state"),
		"status": c.Query("status"),
	}

	chatId := c.Params("chatId")
	data, pagination, err := kschsvc.MessageList(ctx, chatId, page, perPage, filters, sortField, sortOrder)
	if err != nil {
		log.Error().Err(err).Str("chatId", chatId).Msg("❌ Failed to get messages")
		return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to list messages")
	}

	return gmod.SendPagination(c, data, pagination)
}

// MessageUpdate godoc
// @Summary Update a message
// @Description อัปเดตข้อความใน Chat
// @Tags Ksearch/Chats
// @Accept json
// @Produce json
// @Param chatId path string true "Chat ID"
// @Param messageId path string true "Message ID"
// @Param request body kschmod.MessageUpdateRequest true "Message update data"
// @Failure 400 {object} gmod.BaseResponse "Invalid ID format"
// @Failure 404 {object} gmod.BaseResponse "Message not found"
// @Failure 500 {object} gmod.BaseResponse "Internal server error"
// @Router /ksearch/chats/{chatId}/messages/{messageId} [patch]
// @Security BearerAuth
func MessageUpdate(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "github.com/hotkhwan/gateway-api/kschapi", "MessageUpdate", "kschapi", "MessageUpdate")
	defer span.End()

	chatId := c.Params("chatId")
	messageId := c.Params("messageId")

	var req kschmod.MessageUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "invalid request body")
	}

	_, err := kschsvc.MessageUpdate(ctx, chatId, messageId, req)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return httputil.FailNotFound(c, "NOT_FOUND", "message not found")
		}
		if strings.Contains(err.Error(), "invalid id format") {
			return httputil.FailBadRequest(c, "BAD_REQUEST", "Invalid ID format")
		}

		log.Error().Err(err).Str("chatId", chatId).Str("messageId", messageId).Msg("❌ failed to update message")
		return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to update message")
	}

	return c.Status(fiber.StatusOK).JSON(gmod.SuccessMessageResponse{
		Code:    "SUCCESS",
		Message: "Message updated successfully",
		Status:  true,
	})
}
