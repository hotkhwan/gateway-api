// router/kwatch.go
package router

import (
	"klynx/controllers/kwatapi"
	"klynx/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

func RegisterKwatchRoutes(router fiber.Router) {
	router.Route("/kwatch", func(r fiber.Router) {
		r.Use(middleware.AuthBearer())
		r.Post("/syncIboc", middleware.AllowOnly("POST"), kwatapi.SyncIbocWatchlist)
		r.Post("/syncIboc/ids", middleware.AllowOnly("POST"), kwatapi.SyncIbocByIDs)
		r.Post("/syncIbocDev", middleware.AllowOnly("POST"), kwatapi.SyncIbocDevWatchlist)
		r.Get("/watchlist", middleware.AllowOnly("GET"), kwatapi.WatchlistList)
		r.Get("/watchlist/:id", middleware.AllowOnly("GET"), kwatapi.WatchlistGetByID)
		r.Post("/watchlist", middleware.AllowOnly("POST"), kwatapi.WatchlistCreate)
		// r.Post("/watchlist3party", middleware.AllowOnly("POST"), kwatapi.CreateWatchlist3party)
		r.Delete("/watchlist/:id", middleware.AllowOnly("DELETE"), kwatapi.WatchlistDelete)
		r.Patch("/watchlist/:id", middleware.AllowOnly("PATCH"), kwatapi.WatchlistUpdate)
		r.Post("/watchlist/exports", middleware.AllowOnly("POST"), kwatapi.CreateWatchlistExport)
		r.Get("/watchlist/exports/jobs/:id", middleware.AllowOnly("GET"), kwatapi.GetExportJob)
		r.Get("/watchlist/exports/:id", middleware.AllowOnly("GET"), kwatapi.DownloadExport)
		// r.All("/watchlist3Partry/:id", middleware.AllowOnly("DELETE"), kwatapi.DeleteWatchlist3party)
	})
}
