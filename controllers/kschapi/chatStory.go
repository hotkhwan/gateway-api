// controllers/kschapi/chatStory.go
package kschapi

import (
	"errors"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/internal/services/kschsvc"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/kschmod"
	"github.com/hotkhwan/gateway-api/utils"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"strings"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/mongo"
)

// ChatCreate godoc
// @Summary Create a new chat in a project
// @Description สร้าง Chat ใหม่ภายใน Project
// @Tags Ksearch/Chats
// @Accept json
// @Produce json
// @Param body body kschmod.Chat true "Chat info"
// @Success 201 {object} gmod.SuccessMessageCreateResponse
// @Failure 400 {object} gmod.ErrorResponse
// @Failure 500 {object} gmod.ErrorResponse
// @Router /ksearch/chats [post]
// @Security BearerAuth
func ChatCreate(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kschapi", "ChatStory.ChatCreate", "kschapi", "ChatCreate")
	defer end()

	var req kschmod.ChatRequest
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	userId, err := middleware.CurrentUserID(c)
	if err != nil {
		return httputil.FailUnauthorized(c, "UNAUTHORIZED", err.Error())
	}
	chat, err := kschsvc.ChatCreate(ctx, req, userId)
	if err != nil {
		log.Error().Err(err).Msg("❌ create chat failed")

		if strings.Contains(err.Error(), "invalid projectId format") {
			return httputil.FailBadRequest(c, "BAD_REQUEST", "invalid projectId format")
		}

		return httputil.FailInternal(c, "CHAT_CREATE_FAILED", "Failed to create chat")
	}

	return c.Status(fiber.StatusCreated).JSON(gmod.SuccessMessageCreateResponse{
		Code:    "SUCCESS",
		Message: "Chat created successfully",
		Status:  true,
		ID:      chat.ID.Hex(),
	})

}

// ChatList godoc
// @Summary List chats in a project
// @Description ดึงรายการ Chat ทั้งหมดใน Project ที่เลือก
// @Tags Ksearch/Chats
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID"
// @Success 200 {object} gmod.SuccessResponse
// @Failure 500 {object} gmod.ErrorResponse
// @Router /ksearch/projects/{projectId}/chats [get]
// @Security BearerAuth
func ChatList(c fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c, "gateway.kschapi", "ChatStory.ChatList", "kschapi", "ChatList")
	defer end()

	projectId := c.Params("id") // ถ้ามี param id จาก /projects/:id/chats
	if projectId == "" {
		projectId = c.Query("projectId", "") // หรือถ้าใช้ query ก็ยังรองรับ
	}
	userId, err := middleware.CurrentUserID(c)
	if err != nil {
		return httputil.FailUnauthorized(c, "UNAUTHORIZED", err.Error())
	}

	chats, err := kschsvc.ChatList(ctx, projectId, userId)
	if err != nil {
		return httputil.FailInternal(c, "CHAT_LIST_FAILED", "Failed to list chats")
	}
	// if chats == nil {
	// 	chats = []kschmod.Chat{} // ensure เป็น [] ไม่ใช่ nil
	// }
	return c.JSON(kschmod.ChatListResponse{
		BaseResponse: gmod.BaseResponse{
			Code:    "SUCCESS",
			Message: "Chats retrieved successfully",
			Status:  true,
		},
		Details: utils.EmptyIfNil(chats),
		Pagination: gmod.Pagination{
			Page:         1,
			PerPages:     len(chats),
			TotalRecords: len(chats),
			TotalPages:   1,
			SortField:    "createdAt",
			SortOrder:    "desc",
		},
	})

}

// ChatUpdate godoc
// @Summary Update a chat
// @Description อัปเดตข้อมูล Chat
// @Tags Ksearch/Chats
// @Accept json
// @Produce json
// @Param chatId path string true "Chat ID"
// @Param request body kschmod.ChatsUpdateRequest true "Chat update data"
// @Failure 400 {object} gmod.BaseResponse "Invalid chat ID format"
// @Failure 404 {object} gmod.BaseResponse "chat not found"
// @Failure 500 {object} gmod.BaseResponse "Internal server error"
// @Router /ksearch/chats/{chatId} [put]
func ChatUpdate(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kschapi", "ChatStory.ChatUpdate", "kschapi", "ChatUpdate")
	defer end()
	chatId := c.Params("chatId")
	var req kschmod.ChatRequest
	if err := c.Bind().Body(&req); err != nil {
		return httputil.FailBadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	_, err := kschsvc.ChatUpdate(ctx, chatId, kschmod.Chat{
		Name: req.Name,
	})
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return httputil.FailNotFound(c, "NOT_FOUND", "chat not found")
		}
		if strings.Contains(err.Error(), "invalid id format") {
			return httputil.FailBadRequest(c, "BAD_REQUEST", "Invalid chat ID format")
		}

		log.Error().Err(err).Str("chatId", chatId).Msg("❌ failed to get chat")
		return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to get chat")
	}
	return c.Status(fiber.StatusOK).JSON(gmod.SuccessMessageResponse{
		Code:    "SUCCESS",
		Message: "Chat updated successfully",
		Status:  true,
	})
}

// ChatDelete godoc
// @Summary Delete a chat
// @Description ลบ Chat ตาม ID
// @Tags Ksearch/Chats
// @Produce json
// @Param chatId path string true "Chat ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} gmod.BaseResponse "Invalid chat ID format"
// @Failure 404 {object} gmod.BaseResponse "chat not found"
// @Failure 500 {object} gmod.BaseResponse "Internal server error"
// @Router /ksearch/chats/{chatId} [delete]
func ChatDelete(c fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c, "gateway.kschapi", "ChatStory.ChatDelete", "kschapi", "ChatDelete")
	defer end()
	chatId := c.Params("chatId")
	if err := kschsvc.ChatDelete(ctx, chatId); err != nil {
		log.Error().Err(err).Str("chatId", chatId).Msg("❌ failed to get chat")
		switch {
		case errors.Is(err, kschsvc.ErrInvalidObjectID):
			return httputil.FailBadRequest(c, "INVALID_ID", "Invalid chatId format")
		case errors.Is(err, kschsvc.ErrVideoNotFound):
			return httputil.FailNotFound(c, "NOT_FOUND", "Chat not found")
		default:
			return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to delete chat")
		}
	}
	log.Debug().Str("chatId", chatId).Msg("✅ ChatDelete successfully")
	return httputil.MessageOK(c, "Chat deleted successfully")
}
