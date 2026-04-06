// router/iboc.go
package router

import (
	"github.com/hotkhwan/gateway-api/controllers/webhooks/analytic/ibocapi"

	"github.com/gofiber/fiber/v3"
)

func RegisterHookIboc(router fiber.Router) {
	router.Post("/ibochook", ibocapi.HandleIboc)
}
