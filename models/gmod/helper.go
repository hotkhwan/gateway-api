// models/gmod/helper.go
package gmod

import (
	"github.com/gofiber/fiber/v2"
)

// ✅ 1. SUCCESS (200 OK)

// ส่ง JSON 200 สำหรับข้อมูลทั่วไป (struct, map, string, etc.)
func SendSuccess[T any](c *fiber.Ctx, data T) error {
	return c.Status(fiber.StatusOK).JSON(SuccessDetailResponse[T]{
		Details: data,
		Status: true,
	})
}

// ส่ง JSON 200 สำหรับข้อความสำเร็จทั่วไป เช่น PUT / DELETE
func SendMessageOK(c *fiber.Ctx, code, msg string) error {
	return c.Status(fiber.StatusOK).JSON(SuccessMessageResponse{
		Code:    code,
		Message: msg,
		Status:  true,
	})
}

// ส่ง JSON 200 สำหรับผลลัพธ์แบบ pagination ที่ใช้ generic
func SendPagination[T any](c *fiber.Ctx, data []T, pagination Pagination) error {
	// ถ้า data เป็น nil บังคับให้เป็น slice ว่าง
	if data == nil {
		data = []T{}
	}
	return c.Status(fiber.StatusOK).JSON(PaginationDbResponse[T]{
		Details:    data,
		Pagination: pagination,
		Status:     true,
	})
}

func SendPaginationOK[T any](
	c *fiber.Ctx,
	data []T,
	pagination Pagination,
) error {
	if data == nil {
		data = []T{}
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"code":       "SUCCESS",
		"message":    "ok",
		"status":     true,
		"details":    data,
		"pagination": pagination,
	})
}

// ส่ง JSON 200 สำหรับ Dahua camera response
func SendDahuaResponse(c *fiber.Ctx, imgCount int, jsonSample map[string]interface{}) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":     "ok",
		"imgCount":   imgCount,
		"jsonSample": jsonSample,
	})
}

// ส่ง JSON 200 สำหรับ IBOC camera response
func SendIbocResponse(c *fiber.Ctx, jsonSample interface{}) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":     "ok",
		"jsonSample": jsonSample,
	})
}

// ✅ 2. SUCCESS (201 Created)
func SendCreatedMessage(c *fiber.Ctx, code, msg string) error {
	return c.Status(fiber.StatusCreated).JSON(SuccessMessageResponse{
		Code:    code,
		Message: msg,
		Status:  true,
	})
}

func SendCreatedWithID(c *fiber.Ctx, code, msg, id string) error {
	return c.Status(fiber.StatusCreated).JSON(SuccessMessageCreateResponse{
		Code:    code,
		Message: msg,
		Status:  true,
		ID:      id,
	})
}

// ✅ 3. SUCCESS (202 Accepted)
func SendAccepted(c *fiber.Ctx, code, msg string) error {
	return c.Status(fiber.StatusAccepted).JSON(SuccessMessageResponse{
		Code:    code,
		Message: msg,
		Status:  true,
	})
}

func SendAcceptedWithID(c *fiber.Ctx, code, msg, id string) error {
	return c.Status(fiber.StatusAccepted).JSON(SuccessMessageCreateResponse{
		Code:    code,
		Message: msg,
		Status:  true,
		ID:      id,
	})
}
