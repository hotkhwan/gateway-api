// internal/repo/stos3minio/upload.go
package stos3minio

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	minioSDK "github.com/minio/minio-go/v7"
)

func Upload(ctx context.Context, bucketKey string, isPublic bool, objectKey string, fileBytes []byte, contentType string) (string, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/stos3minio",
		"files.Upload",
		"stos3minio", "Upload",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	bucketName, ok := pickBucket(bucketKey, isPublic)
	if !ok {
		return "", fmt.Errorf("undefined S3 bucket key: %s", bucketKey)
	}

	start := time.Now()
	_, err := config.S3Client.PutObject(
		ctx, bucketName, objectKey,
		bytes.NewReader(fileBytes), int64(len(fileBytes)),
		minioSDK.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		log.Error().Str("bucket", bucketName).Str("objectKey", objectKey).Err(err).Msg("❌ PutObject failed")
		return "", err
	}

	url := fmt.Sprintf("%s/%s/%s", config.S3BaseURL, bucketName, objectKey)
	log.Debug().
		Str("bucket", bucketName).
		Str("objectKey", objectKey).
		Int("bytes", len(fileBytes)).
		Str("contentType", contentType).
		Dur("took", time.Since(start)).
		Msg("✅ Uploaded to S3")
	return url, nil
}

func UploadWithMeta(ctx context.Context, bucketKey string, isPublic bool, objectKey string, fileBytes []byte, contentType string, metadata map[string]string) (string, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/stos3minio",
		"files.UploadWithMeta",
		"stos3minio", "UploadWithMeta",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	bucketName, ok := pickBucket(bucketKey, isPublic)
	if !ok {
		return "", fmt.Errorf("undefined S3 bucket key: %s", bucketKey)
	}

	start := time.Now()
	_, err := config.S3Client.PutObject(
		ctx, bucketName, objectKey,
		bytes.NewReader(fileBytes), int64(len(fileBytes)),
		minioSDK.PutObjectOptions{ContentType: contentType, UserMetadata: metadata},
	)
	if err != nil {
		log.Error().Str("bucket", bucketName).Str("objectKey", objectKey).Err(err).Msg("❌ PutObject failed")
		return "", err
	}

	url := fmt.Sprintf("%s/%s/%s", config.S3BaseURL, bucketName, objectKey)
	log.Debug().
		Str("bucket", bucketName).
		Str("objectKey", objectKey).
		Int("bytes", len(fileBytes)).
		Interface("metadata", metadata).
		Str("contentType", contentType).
		Dur("took", time.Since(start)).
		Msg("✅ Uploaded to S3 with metadata")
	return url, nil
}

func pickBucket(bucketKey string, isPublic bool) (string, bool) {
	if isPublic {
		val, ok := config.S3PublicBuckets[bucketKey]
		return val, ok
	}
	val, ok := config.S3PrivateBuckets[bucketKey]
	return val, ok
}
