// router/ksearch.go
package router

import (
	"klynx/controllers/kschapi"
	"klynx/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterKsearchRoutes(router fiber.Router) {
	router.Route("/ksearch", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		// Projects
		r.Post("/projects", middleware.AllowOnly("POST"), kschapi.ProjectCreate)
		r.Get("/projects", middleware.AllowOnly("GET"), kschapi.ProjectList)
		r.Get("/projects/:projectId", middleware.AllowOnly("GET"), kschapi.ProjectGetByID)
		r.Patch("/projects/:projectId", middleware.AllowOnly("PATCH"), kschapi.ProjectUpdate)
		r.Delete("/projects/:projectId", middleware.AllowOnly("DELETE"), kschapi.ProjectDelete)

		// Chats
		r.Get("/chats", middleware.AllowOnly("GET"), kschapi.ChatList)
		r.Post("/chats", middleware.AllowOnly("POST"), kschapi.ChatCreate)
		r.Patch("/chats/:chatId", middleware.AllowOnly("PATCH"), kschapi.ChatUpdate)
		r.Delete("/chats/:chatId", middleware.AllowOnly("DELETE"), kschapi.ChatDelete)
		r.Get("/projects/:chatId/chats", middleware.AllowOnly("GET"), kschapi.ChatList)

		// Messages
		r.Post("/chats/messages", middleware.AllowOnly("POST"), kschapi.MessageCreateStream)
		r.Post("/chats/:chatId/messages", middleware.AllowOnly("POST"), kschapi.MessageCreateStream)
		r.Get("/chats/:chatId/messages", middleware.AllowOnly("GET"), kschapi.MessageList)

		// Video search
		r.Get("/video", middleware.AllowOnly("GET"), kschapi.VideoList)                  // 501 (stub)
		r.Get("/video/:videoId", middleware.AllowOnly("GET"), kschapi.VideoGetByID)      // 501 (stub)
		r.Post("/video", middleware.AllowOnly("POST"), kschapi.VideoCreate)              // ✅ ใช้งานจริง
		r.Delete("/video/:videoId", middleware.AllowOnly("DELETE"), kschapi.VideoDelete) // 501 (stub)
		// r.Patch("/video/:id", middleware.AllowOnly("PATCH"), kschapi.VideoUpdate)   // 501 (stub)
	})
}
