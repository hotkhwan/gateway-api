// router/group.go
package router

import (
	"klynx/controllers/memapi"
	"klynx/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterMemberRoutes(router fiber.Router) {
	router.Route("/member", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Get("", middleware.AllowOnly("GET"), memapi.ListMembers)
		r.Get("/:id", middleware.AllowOnly("GET"), memapi.MemberGetByID)
		r.Get("/user/:id", middleware.AllowOnly("GET"), memapi.MemberGetByUserID)
		r.Get("/group/:groupId", middleware.AllowOnly("GET"), memapi.MemberGetByGroupID)
		r.Post("/:groupId", middleware.AllowOnly("POST"), memapi.CreateMember2)
		r.Patch("/:id", middleware.AllowOnly("PATCH"), memapi.UpdateMember)
		r.Delete("/:id", middleware.AllowOnly("DELETE"), memapi.DeleteMember)

	})
}
