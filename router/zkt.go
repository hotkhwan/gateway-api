// router/zkt.go
package router

import (
	"github.com/hotkhwan/gateway-api/controllers/mediapi"
	"github.com/hotkhwan/gateway-api/controllers/webhooks/streamzkt"
	"github.com/hotkhwan/gateway-api/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterHookzkt(router fiber.Router) {
	router.Route("/zkthook", func(r fiber.Router) {
		r.Post("/onPublish", streamzkt.OnPublish)
		r.Post("/onPlay", streamzkt.OnPlay)
		r.Post("/onStreamNoneReader", streamzkt.OnStreamNoneReader)
		r.Post("/onStreamNotFound", streamzkt.OnStreamNotFound)
	})
}

func RegisterMedia(router fiber.Router) {

	router.Route("/media", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Get("/config", mediapi.ListConfig)
		r.Get("/stream", mediapi.ListStream)
		r.Post("/stream", mediapi.CreateStream)
		r.Delete("/stream/:streamId", mediapi.DeleteStream)
	})
}
