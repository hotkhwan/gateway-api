// router/group.go
package router

import (
	"klynx/controllers/grpapi"
	"klynx/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterGroupRoutes(router fiber.Router) {
	router.Route("/groups", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Get("", middleware.AllowOnly("GET"), grpapi.ListGroups)
		r.Get("/search", middleware.AllowOnly("GET"), grpapi.SearchGroupsTree)
		r.Get("/tree", middleware.AllowOnly("GET"), grpapi.GetGroupTree)
		r.Get("/subgroups", middleware.AllowOnly("GET"), grpapi.ListSubGroups)
		r.Get("/members", middleware.AllowOnly("GET"), grpapi.ListGroupMembers)
		r.Get("/devices", middleware.AllowOnly("GET"), grpapi.ListGroupDevices)
		r.Get("/roles", middleware.AllowOnly("GET"), grpapi.ListGroupRoles)
		r.Post("", middleware.AllowOnly("POST"), grpapi.CreateGroup)
		// เพิ่ม r.Get เพื่อเรียก parent-child ในขั้นต่อไป
		r.Patch("/:id", middleware.AllowOnly("PATCH"), grpapi.UpdateGroup)
		r.Delete("/:id", middleware.AllowOnly("DELETE"), grpapi.DeleteGroup)
	})
}
