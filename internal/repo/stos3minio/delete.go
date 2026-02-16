// internal/repo/stos3minio/delete.go
package stos3minio

import (
	"context"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"os"
	"time"
)

// DeleteFile รับได้ทั้ง full URL และ key ล้วน
// - ถ้าเป็น URL: parse แล้วไปใช้ DeleteByKey เพื่อให้ได้ normalize/log เดิม
// - ถ้าเป็น key: ใช้ bucket จาก env (S3_WATCHLIST_BUCKET) หรือ fallback "kwatch"
func DeleteFile(ctx context.Context, target string) error {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/stos3minio",
		"s3.DownloadByKey",
		"stos3minio", "DownloadByKey",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var bucket, objectKey string
	var err error

	if LooksLikeURL(target) {
		bucket, objectKey, err = parseS3URL(target)
		if err != nil {
			log.Error().Str("target", target).Err(err).Msg("❌ parseS3URL failed")
			return err
		}
		return DeleteByKey(ctx, bucket, objectKey)
	}

	// key ล้วน
	bucket = os.Getenv("S3_WATCHLIST_BUCKET")
	if bucket == "" {
		bucket = "kwatch" // ปรับตามดีฟอลต์ของคุณ
	}
	return DeleteByKey(ctx, bucket, target)
}

// (option) เผื่ออยากเรียกตรง ๆ
func DeleteByURL(ctx context.Context, fullURL string) error {
	ctx, end, _ := traceutil.StartLite(
		ctx,
		"github.com/hotkhwan/gateway-api/stos3minio",
		"files.DownloadByKey",
		"stos3minio", "DownloadByKey",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	bucket, objectKey, err := parseS3URL(fullURL)
	if err != nil {
		return err
	}
	return DeleteByKey(ctx, bucket, objectKey)
}

// NOTE: DeleteByKey(ctx, bucket, objectKey) ของคุณยังคงใช้ได้เหมือนเดิม
// และยังแนะนำให้คงไว้เพื่อเคสที่รู้อยู่แล้วว่าบัคเก็ตไหน
