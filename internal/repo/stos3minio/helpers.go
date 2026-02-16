// internal/repo/stos3minio/helpers.go
package stos3minio

import (
	"bytes"
	"context"
	"github.com/hotkhwan/gateway-api/config"

	minio "github.com/minio/minio-go/v7"
)

func UploadBytes(ctx context.Context, bucket, key, contentType string, data []byte) error {
	_, err := config.S3Client.PutObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	return err
}
