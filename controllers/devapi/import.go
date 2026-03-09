// controllers/devapi/import.go
package devapi

import (
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/services/devsvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// DeviceTemplate godoc
// @Summary      Upload device template
// @Description  Upload and import device list from CSV or XLSX file
// @Tags         Devices
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        file formData file true "Device Template File (.csv or .xlsx)"
// @Success      200 {object} gmod.SuccessMessageResponse
// @Failure      400 {object} gmod.ErrorResponse
// @Failure      401 {object} gmod.ErrorResponse
// @Failure      500 {object} gmod.ErrorResponse
// @Router       /devices/import [post]
// @Security BearerAuth
func DeviceTemplate(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.devapi", "DeviceTemplate", "devapi", "DeviceTemplate")
	defer end()
	log.Info().
		Msg("📥 [DeviceTemplate] Incoming request")

	form, err := c.MultipartForm()
	if err != nil || len(form.File["file"]) == 0 {
		log.Warn().
			Err(err).Msg("❌ No file in request")
		return httputil.FailBadRequest(c, "No file attached to the request")
	}

	fileHeader := form.File["file"][0]
	ext := strings.ToLower(strings.TrimPrefix(strings.ToLower(fileHeader.Filename[strings.LastIndex(fileHeader.Filename, ".")+1:]), "."))
	file, err := fileHeader.Open()
	if err != nil {
		log.Error().
			Err(err).Msg("❌ Cannot open uploaded file")
		return httputil.FailInternal(c, "Unable to read uploaded file")
	}
	defer file.Close()

	var devices []map[string]interface{}
	var duplicates, invalid []string

	switch ext {
	case "csv":
		devices, duplicates, invalid, err = devsvc.ParseCSVFile(ctx, file)
	case "xlsx":
		devices, duplicates, invalid, err = devsvc.ParseXLSXFile(ctx, file)
	default:
		log.Warn().
			Str("ext", ext).Msg("❌ Unsupported file type")
		return httputil.FailBadRequest(c, "Unsupported file type")
	}

	if err != nil {
		log.Error().
			Err(err).Msg("❌ Parse file error")
		return httputil.FailBadRequest(c, err.Error())
	}

	if len(invalid) > 0 {
		msg := fmt.Sprintf("Missing required fields in row(s): %s", strings.Join(invalid, ", "))
		log.Warn().
			Msg(msg)
		return httputil.FailBadRequest(c, msg)
	}
	if len(duplicates) > 0 {
		msg := fmt.Sprintf("Duplicate IP addresses: %s", strings.Join(duplicates, ", "))
		log.Warn().
			Msg(msg)
		return httputil.FailBadRequest(c, msg)
	}

	// 🔍 Check IPs already in DB
	existing, err := devsvc.FindDuplicateIPs(ctx, devices)
	if err != nil {
		log.Error().
			Err(err).Msg("❌ Error checking existing IPs")
		return httputil.FailInternal(c, "Failed to check IP duplication")
	}
	if len(existing) > 0 {
		msg := fmt.Sprintf("Duplicate IPs found in DB: %s", strings.Join(existing, ", "))
		log.Warn().
			Msg(msg)
		return httputil.FailBadRequest(c, msg)
	}

	// ✅ Insert
	inserted, err := devsvc.InsertDevices(ctx, devices)
	if err != nil {
		log.Error().
			Err(err).Msg("❌ Mongo insert failed")
		return httputil.FailInternal(c, "MongoDB insert failed")
	}

	log.Info().
		Int("count", inserted).Msg("✅ Devices inserted successfully")
	return httputil.MessageOK(c, "Create devices successfully")
}
