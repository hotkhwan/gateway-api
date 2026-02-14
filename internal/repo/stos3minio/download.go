// internal/repo/stos3minio/download.go
package stos3minio

import (
	"context"
	"io"
	"path/filepath"
	"time"

	"klynx/config"
	"klynx/utils/traceutil"

	"github.com/minio/minio-go/v7"
)

func DownloadByKey(ctx context.Context, bucket, key string) ([]byte, string, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/stos3minio",
		"files.DownloadByKey",
		"stos3minio", "DownloadByKey",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	obj, err := config.S3Client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		log.Error().Err(err).Str("bucket", bucket).Str("objectKey", key).Msg("❌ GetObject failed")
		return nil, "", err
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		log.Error().Err(err).Str("bucket", bucket).Str("objectKey", key).Msg("❌ ReadAll failed")
		return nil, "", err
	}
	filename := filepath.Base(key)
	return data, filename, nil
}
