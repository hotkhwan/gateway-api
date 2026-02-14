// internal/services/kschsvc/videoGetById.go
package kschsvc

import (
	"context"
	"errors"
	"os"
	"time"

	"klynx/internal/repo/stomongo"
	"klynx/internal/repo/stos3minio"
	"klynx/models/kschmod"
	"klynx/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func VideoGetByID(ctx context.Context, id string) (kschmod.VideoResponse, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/kschsvc",       // tracerName
		"search.VideoGetByID", // spanName (แนะนำให้ prefix ด้วยแพ็กเกจ)
		"kschsvc", "VideoGetByID",
	)
	defer end()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var result kschmod.VideoResponse

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return result, errors.New("invalid id format")
	}

	filter := bson.M{"_id": objID}
	if err := stomongo.FindOne(ctx, videoColl, filter, &result); err != nil {
		return result, err
	}

	// presign video_url
	if result.VideoKey != "" {
		bucket := os.Getenv("S3_KSEARCH_BUCKET")
		if bucket == "" {
			bucket = os.Getenv("S3_BUCKET")
		}
		if bucket == "" {
			bucket = videoBucket
		}
		if bucket != "" {
			url, err := stos3minio.PresignOnce(ctx, bucket, result.VideoKey, 0)
			if err != nil {
				log.Warn().Err(err).Str("key", result.VideoKey).Msg("⚠️ presign failed")
			} else {
				result.VideoURL = url
			}
		}
	}

	return result, nil
}
