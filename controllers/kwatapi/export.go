// controllers/kwatapi/export.go
package kwatapi

import (
	"net/http"

	"github.com/gofiber/fiber/v2"

	"klynx/internal/services/kwatsvc"
	"klynx/models/kwatmod"
	"klynx/utils/traceutil"
)

// @Summary  Create watchlist export job
// @Tags     Kwatch/Watchlist
// @Produce  json
// @Param    search  query string false "search by idcard/fullName/personKey"
// @Param    from    query string false "YYYY-MM-DD"
// @Param    to      query string false "YYYY-MM-DD"
// @Param    limit   query int    false "max 20000 (default 5000)"
// @Param    onlyIds query string false "comma separated _id"
// @Success  202 {object} map[string]any
// @Router   /watchlists/exports [post]
// @Security BearerAuth
func CreateWatchlistExport(c *fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c.Context(), "klynx/kwatapi", "CreateExport", "kwatapi", "export-create")
	defer end()

	var p kwatmod.ExportWatchlistParams
	if err := c.QueryParser(&p); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	jobID, err := kwatsvc.StartWatchlistExport(ctx, p)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(http.StatusAccepted).JSON(fiber.Map{
		"status":  true,
		"code":    "ACCEPTED",
		"message": "Export job accepted",
		"data": fiber.Map{
			"jobId": jobID,
		},
	})
}

// @Summary  Get export job status
// @Tags     Kwatch/Watchlist
// @Produce  json
// @Param    id path string true "jobId"
// @Success  200 {object} kwatmod.ExportJob
// @Router   /watchlist/exports/jobs/{id} [get]
// @Security BearerAuth
func GetExportJob(c *fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c.Context(), "klynx/kwatapi", "GetJob", "kwatapi", "export-get")
	defer end()

	id := c.Params("id")
	job, err := kwatsvc.GetJob(ctx, id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.JSON(job)
}

// @Summary  Download exported watchlist zip
// @Tags     Kwatch/Watchlist
// @Produce  application/zip
// @Param    id path string true "jobId"
// @Success  302 {string} string "redirect to S3 URL"
// @Router   /watchlist/exports/{id} [get]
// @Security BearerAuth
func DownloadExport(c *fiber.Ctx) error {
	ctx, end, _ := traceutil.StartLite(c.Context(), "klynx/kwatapi", "Download", "kwatapi", "export-download")
	defer end()

	id := c.Params("id")
	job, err := kwatsvc.GetJob(ctx, id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	if job.Status != kwatmod.ExportStatusSucceeded || job.Result == nil || job.Result.URL == "" {
		return fiber.NewError(fiber.StatusAccepted, "file is not ready yet")
	}
	c.Set("Location", job.Result.URL)
	return c.Status(http.StatusFound).Send(nil) // 302
}
