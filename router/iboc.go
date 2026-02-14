// router/iboc.go
package router

import (
	"klynx/controllers/webhooks/analytic/ibocapi"

	"github.com/gofiber/fiber/v2"
)

func RegisterHookIboc(router fiber.Router) {
	router.Post("/ibochook", ibocapi.HandleIboc)
}
