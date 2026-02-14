// internal/apis/kschapi/videoDelete.go
package kschapi

import (
	"errors"

	"klynx/internal/services/kschsvc"
	"klynx/models/gmod"
	"klynx/utils/httputil"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// videoDelete godoc
// @Summary Delete a Video by ID
// @Description Delete video document and its object in S3 by MongoDB _id
// @Tags Ksearch/Video
// @Produce json
// @Param id path string true "Video MongoDB _id"
// @Success 200 {object} gmod.SuccessMessageResponse
// @Failure 400 {object} gmod.ErrorMessageResponse "Invalid ObjectID"
// @Failure 404 {object} gmod.ErrorMessageResponse "Video not found"
// @Failure 500 {object} gmod.ErrorMessageResponse "Internal Server Error"
// @Router /ksearch/video/{id} [delete]
// @Security BearerAuth
func VideoDelete(c *fiber.Ctx) error {
	idStr := c.Params("videoId")
	objID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		return httputil.FailBadRequest(c, "INVALID_ID", "Invalid video ID format")
	}

	if err := kschsvc.VideoDelete(c.Context(), objID); err != nil {
		switch {
		case errors.Is(err, kschsvc.ErrInvalidObjectID):
			return httputil.FailBadRequest(c, "INVALID_ID", "Invalid video ID format")
		case errors.Is(err, kschsvc.ErrVideoNotFound):
			return httputil.FailNotFound(c, "NOT_FOUND", "Video not found")
		default:
			return httputil.FailInternal(c, "INTERNAL_ERROR", "Failed to delete video")
		}
	}

	return gmod.SendMessageOK(c, "SUCCESS", "Video deleted successfully")
}
