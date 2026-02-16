// router\face.go
package router

import (
	"github.com/hotkhwan/gateway-api/controllers/webhooks/analytic/camDahuaapi"

	"github.com/gofiber/fiber/v2"
)

func RegisterFacesCCTVRoutes(router fiber.Router) {
	router.Post("/dahuahook", camDahuaapi.HandleDahua)
}
