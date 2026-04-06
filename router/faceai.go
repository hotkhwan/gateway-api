// router\face.go
package router

import (
	"github.com/hotkhwan/gateway-api/controllers/webhooks/analytic/camDahuaapi"

	"github.com/gofiber/fiber/v3"
)

func RegisterFacesCCTVRoutes(router fiber.Router) {
	router.Post("/dahuahook", camDahuaapi.HandleDahua)
}
