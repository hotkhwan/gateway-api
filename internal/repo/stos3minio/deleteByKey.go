// internal/repo/stos3minio/deleteByKey.go
package stos3minio

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	minio "github.com/minio/minio-go/v7"
)

func normalizeKey(key string) string {
	if strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://") {
		parts := strings.SplitN(key, "://", 2)
		path := parts[1]
		if i := strings.Index(path, "/"); i > -1 {
			path = path[i+1:]
		}
		if q := strings.Index(path, "?"); q > -1 {
			path = path[:q]
		}
		if i := strings.Index(path, "/"); i > -1 {
			return strings.TrimLeft(path[i+1:], "/")
		}
		return strings.TrimLeft(path, "/")
	}
	return strings.TrimLeft(key, "/")
}

func DeleteByKey(ctx context.Context, bucket, objectKey string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, endSpan, log := traceutil.StartLite(
		ctx, "github.com/hotkhwan/gateway-api/stos3minio", "files.DeleteByKey",
		"stos3minio", "DeleteByKey",
	)
	defer endSpan()

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// ✅ ถ้า client ยังไม่พร้อม ให้ error ชัดเจน (ตอนนี้คุณลบไม่ไปเพราะกรณีนี้บ่อย)
	if config.S3Client == nil {
		log.Error().Msg("❌ S3Client is nil — did you call config.InitS3()? (check this service's env)")
		return fmt.Errorf("s3 client not initialized")
	}
	endpoint := ""
	if u := config.S3Client.EndpointURL(); u != nil {
		endpoint = u.String()
	}

	key := normalizeKey(objectKey)
	log.Debug().
		Str("endpoint", endpoint).
		Str("bucket", bucket).
		Str("rawKey", objectKey).
		Str("normKey", key).
		Msg("🔎 S3 delete debug")

	// --- Stat ก่อนลบ (เพื่อ log etag/time ยืนยันว่ามีจริง) ---
	if st, err := config.S3Client.StatObject(ctx, bucket, key, minio.StatObjectOptions{}); err == nil {
		log.Info().
			Str("bucket", bucket).
			Str("objectKey", key).
			Str("etag", st.ETag).
			Time("lastModified", st.LastModified).
			Msg("ℹ️ Object exists before delete")
	} else {
		resp := minio.ToErrorResponse(err)
		if resp.Code == "NoSuchKey" || resp.StatusCode == 404 {
			log.Info().Str("bucket", bucket).Str("objectKey", key).Msg("ℹ️ Object not found before delete (already gone)")
		} else {
			log.Warn().Err(err).Str("bucket", bucket).Str("objectKey", key).Msg("⚠️ StatObject before delete failed")
		}
	}

	// --- ลบ ---
	start := time.Now()
	if err := config.S3Client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
		resp := minio.ToErrorResponse(err)
		if resp.Code == "NoSuchKey" || resp.StatusCode == 404 {
			log.Info().Str("bucket", bucket).Str("objectKey", key).Msg("ℹ️ object not found, treat as deleted")
			return nil
		}
		log.Error().Err(err).Str("bucket", bucket).Str("objectKey", key).Dur("took", time.Since(start)).Msg("❌ Failed to delete object")
		return err
	}
	log.Info().Str("bucket", bucket).Str("objectKey", key).Dur("took", time.Since(start)).Msg("🗑 S3 RemoveObject OK")

	// --- Stat หลังลบ (ถ้า MinIO ไม่มี versioning จะ 404) ---
	if _, err := config.S3Client.StatObject(ctx, bucket, key, minio.StatObjectOptions{}); err != nil {
		resp := minio.ToErrorResponse(err)
		if resp.Code == "NoSuchKey" || resp.StatusCode == 404 {
			log.Info().Str("bucket", bucket).Str("objectKey", key).Msg("✅ Confirmed deleted (Stat 404)")
			return nil
		}
		log.Warn().Err(err).Str("bucket", bucket).Str("objectKey", key).Msg("⚠️ StatObject after delete returned unexpected error")
	} else {
		// เจอไฟล์หลังลบ → ชวนตรวจ endpoint/bucket/key หรือ cache
		log.Warn().
			Str("bucket", bucket).
			Str("objectKey", key).
			Str("endpoint", endpoint).
			Msg("⚠️ Object still stat-able after delete — check endpoint/bucket/key mismatch or caching")
	}
	return nil
}
