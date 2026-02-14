// controllers/atapi/stream.go
package atapi

import (
	"strconv"

	"klynx/internal/services/atasvc"
	"klynx/utils/httputil"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
)

// StreamDetails ใช้เป็น details สำหรับตอบกลับ stream url
type StreamDetails struct {
	ChannelID int64  `json:"channelId" example:"42"`
	Name      string `json:"name" example:"Entrance"`
	Zone      string `json:"zone" example:"AT"`
	SN        string `json:"sn" example:"60038008431d6040"`
	RTSPURL   string `json:"rtspUrl" example:"rtsp://admin:***@172.16.10.151/axis-media/media.amp"`
	WSSFlvURL string `json:"wssFlvUrl" example:"wss://atanywhere.ddns.net:8081/media-api/ws/flv?src=..."` // flv wss url
}

// StreamResponse รูปแบบ response ตาม standard: code/message/status + details
type StreamResponse struct {
	Code    string        `json:"code" example:"SUCCESS"`
	Message string        `json:"message" example:"OK"`
	Status  bool          `json:"status" example:"true"`
	Details StreamDetails `json:"details"`
}

// GetStreamWSS godoc
// @Summary      Get ATA FLV WebSocket stream URL
// @Description  Return WSS FLV URL for given ATA channelId so frontend can play FLV over WebSocket.
// @Tags         ATA
// @Accept       json
// @Produce      json
// @Param        channelId path int true "ATA channel ID"
// @Success      200 {object} atapi.StreamResponse "Stream URL info"
// @Failure      400 {object} gmod.ErrorMessageResponse "Bad Request – invalid or missing channelId"
// @Failure      502 {object} gmod.ErrorMessageResponse "Bad Gateway – ATA upstream error"
// @Failure      500 {object} gmod.ErrorMessageResponse "Internal Server Error – config or build failed"
// @Router       /ata/stream/{channelId} [get]
// @Security     BearerAuth
func GetStreamWSS(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("klynx/atapi")
	ctx, span := tracer.Start(ctx, "ATA.GetStreamWSS")
	defer span.End()

	channelIDStr := c.Params("channelId")
	if channelIDStr == "" {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "missing channelId")
	}

	chID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil || chID <= 0 {
		return httputil.FailBadRequest(c, "INVALID_CHANNEL_ID", "invalid channelId")
	}

	client, err := atasvc.NewClientFromEnv()
	if err != nil {
		return httputil.FailInternal(c, "ATA_CONFIG_ERROR", "Failed to load ATA config from environment variables")
	}

	token, err := client.Login(ctx)
	if err != nil {
		return httputil.FailInternal(c, "ATA_LOGIN_FAILED", err.Error())
	}

	ch, err := client.GetChannelDetail(ctx, token, chID)
	if err != nil {
		return httputil.FailInternal(c, "ATA_GET_CHANNEL_FAILED", err.Error())
	}

	wsURL, err := client.BuildFLVWSURL(ch.SN, ch.ID)
	if err != nil {
		return httputil.FailInternal(c, "ATA_BUILD_WSS_FAILED", err.Error())
	}

	resp := StreamResponse{
		Code:    "SUCCESS",
		Message: "OK",
		Status:  true,
		Details: StreamDetails{
			ChannelID: ch.ID,
			Name:      ch.Name,
			Zone:      ch.Zone,
			SN:        ch.SN,
			RTSPURL:   ch.MainUrl,
			WSSFlvURL: wsURL,
		},
	}

	return c.JSON(resp)
}
