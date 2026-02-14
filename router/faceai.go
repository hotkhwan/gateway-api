// router\face.go
package router

import (
	"klynx/controllers/webhooks/analytic/camDahuaapi"

	"github.com/gofiber/fiber/v2"
)

func RegisterFacesCCTVRoutes(router fiber.Router) {
	router.Post("/dahuahook", camDahuaapi.HandleDahua)
}
