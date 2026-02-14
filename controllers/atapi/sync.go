// controllers/atapi/sync.go
package atapi

import (
	"net/http"

	"klynx/internal/services/devsync"
	"klynx/models/devsyncmod"
	"klynx/models/gmod"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// SyncRequest เผื่อจะใช้ในอนาคต (ตอนนี้ body ว่างได้)
type SyncRequest struct {
	// ถ้าจะ restrict device id ทีหลังค่อยเติม
	// DeviceId *int64 `json:"deviceId,omitempty"`
}

// DeviceSyncFromSVMS godoc
// @Summary      Sync ATA devices & channels from Edge AI to Klynx
// @Description  Calls ATA Edge AI API to fetch devices and channels then upsert into `camera` collection.
// @Tags         ATA
// @Accept       json
// @Produce      json
// @Param        body body atapi.SyncRequest false "Optional sync options"
// @Success      200 {object} atapi.ATASyncResponse "Sync result"
// @Failure      502 {object} gmod.ErrorMessageResponse "Bad Gateway – ATA upstream error"
// @Failure      500 {object} gmod.ErrorMessageResponse "Internal Server Error – config or DB error"
// @Router       /ata/svms/sync [post]
// @Security     BearerAuth
func DeviceSyncFromSVMS(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/atapi")
	ctx, span := tracer.Start(ctx, "ATA.DeviceSyncFromSVMS")
	defer span.End()

	// ถ้าในอนาคตอยาก parse body:
	// var req SyncRequest
	// if len(c.Body()) > 0 {
	//     if err := c.BodyParser(&req); err != nil {
	//         return c.Status(http.StatusBadRequest).JSON(gmod.ErrorMessageResponse{
	//             Code:    "BAD_REQUEST",
	//             Message: "invalid body: " + err.Error(),
	//             Status:  false,
	//         })
	//     }
	// }

	res, err := devsync.SyncDevicesAndChannelsDefault(ctx)
	if err != nil {
		return c.Status(http.StatusBadGateway).JSON(gmod.ErrorMessageResponse{
			Code:    "ATA_SYNC_FAILED",
			Message: err.Error(),
			Status:  false,
		})
	}

	return c.Status(http.StatusOK).JSON(devsyncmod.ATASyncResponse{
		Code:    "ATA_SYNC_OK",
		Message: "ATA devices/channels synced",
		Status:  true,
		Detail:  *res,
	})
}
