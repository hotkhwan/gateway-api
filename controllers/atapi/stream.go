// controllers/atapi/stream.go
package atapi

import (
	"strconv"

	"github.com/hotkhwan/gateway-api/internal/services/atasvc"
	"github.com/hotkhwan/gateway-api/utils/httputil"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/gofiber/fiber/v2"
)

// StreamDetails is the details payload for stream URL responses
type StreamDetails struct {
	ChannelID int64  `json:"channelId" example:"42"`
	Name      string `json:"name" example:"Entrance"`
	Zone      string `json:"zone" example:"AT"`
	SN        string `json:"sn" example:"60038008431d6040"`
	RTSPURL   string `json:"rtspUrl" example:"rtsp://admin:***@172.16.10.151/axis-media/media.amp"`
	WSSFlvURL string `json:"wssFlvUrl" example:"wss://atanywhere.ddns.net:8081/media-api/ws/flv?src=..."`
}

// GetStreamWSS godoc
// @Summary      Get ATA FLV WebSocket stream URL
// @Description  Return WSS FLV URL for given ATA channelId so frontend can play FLV over WebSocket.
// @Tags         ATA
// @Accept       json
// @Produce      json
// @Param        channelId path int true "ATA channel ID"
// @Success      200 {object} gmod.SuccessDataResponse "Stream URL info"
// @Failure      400 {object} gmod.ApiErrorResponse "Bad Request - invalid or missing channelId"
// @Failure      502 {object} gmod.ApiErrorResponse "Bad Gateway - ATA upstream error"
// @Failure      500 {object} gmod.ApiErrorResponse "Internal Server Error - config or build failed"
// @Router       /ata/stream/{channelId} [get]
// @Security     BearerAuth
func GetStreamWSS(c *fiber.Ctx) error {
	ctx, end, log := traceutil.StartLite(c.UserContext(), "gateway.atapi", "ata.GetStreamWSS", "atapi", "GetStreamWSS")
	defer end()

	channelIDStr := c.Params("channelId")
	if channelIDStr == "" {
		return httputil.FailBadRequest(c, "missing channelId")
	}

	chID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil || chID <= 0 {
		return httputil.FailBadRequest(c, "invalid channelId")
	}

	client, err := atasvc.NewClientFromEnv()
	if err != nil {
		log.Error().Err(err).Msg("[GetStreamWSS] failed to load ATA config")
		return httputil.FailInternal(c, "Failed to load ATA config from environment variables")
	}

	token, err := client.Login(ctx)
	if err != nil {
		log.Error().Err(err).Msg("[GetStreamWSS] ATA login failed")
		return httputil.FailInternal(c, err.Error())
	}

	ch, err := client.GetChannelDetail(ctx, token, chID)
	if err != nil {
		log.Error().Err(err).Msg("[GetStreamWSS] get channel detail failed")
		return httputil.FailInternal(c, err.Error())
	}

	wsURL, err := client.BuildFLVWSURL(ch.SN, ch.ID)
	if err != nil {
		log.Error().Err(err).Msg("[GetStreamWSS] build WSS URL failed")
		return httputil.FailInternal(c, err.Error())
	}

	return httputil.Ok(c, StreamDetails{
		ChannelID: ch.ID,
		Name:      ch.Name,
		Zone:      ch.Zone,
		SN:        ch.SN,
		RTSPURL:   ch.MainUrl,
		WSSFlvURL: wsURL,
	})
}
