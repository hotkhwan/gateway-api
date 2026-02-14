// internal/repo/stos3minio/presign.go
package stos3minio

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"klynx/config"
	"klynx/models/repomod"
	"klynx/utils/traceutil"
)

// getExpiryFromEnv อ่าน S3_EXPIRY แล้วคืนค่า time.Duration
// รองรับ "15m", "2h" หรือเลขล้วน (นาที). ค่า default = 1 minute.
func getExpiryFromEnv() time.Duration {
	const def = time.Minute
	raw := os.Getenv("S3_EXPIRY")
	if raw == "" {
		return def
	}
	// 1) ลอง parse เป็น time.ParseDuration ก่อน (เช่น "15m", "1h30m")
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	// 2) ถ้าเป็นเลขล้วน ให้ตีความเป็น "นาที"
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return time.Duration(n) * time.Minute
	}
	// 3) fallback
	return def
}

// resolveExpiry ใช้ค่าที่ส่งเข้ามา; ถ้าเป็น 0 ให้ไปอ่านจาก ENV
func resolveExpiry(expiry time.Duration) time.Duration {
	if expiry <= 0 {
		return getExpiryFromEnv()
	}
	return expiry
}

func PresignOnce(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	ctx, end, log := traceutil.StartLite(
		ctx,
		"klynx/stos3minio",
		"files.PresignOnce",
		"stos3minio", "PresignOnce",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if key == "" {
		return "", nil
	}

	expiry = resolveExpiry(expiry)

	start := time.Now()
	u, err := config.S3Client.PresignedGetObject(ctx, bucket, key, expiry, nil)
	if err != nil {
		log.Error().Str("bucket", bucket).Str("objectKey", key).Err(err).Msg("❌ PresignedGetObject failed")
		return "", err
	}
	log.Info().
		Str("bucket", bucket).
		Str("objectKey", key).
		Dur("expiry", expiry).
		Dur("took", time.Since(start)).
		Msg("✅ Presigned URL generated")
	return u.String(), nil
}

// PresignMany: ถ้า expiry == 0 จะไปอ่าน ENV เช่นกัน
func PresignMany(ctx context.Context, client repomod.Presigner, bucket string, keys []string, expiry time.Duration, maxWorkers int) []repomod.PresignItem {
	out := make([]repomod.PresignItem, len(keys))
	if len(keys) == 0 {
		return out
	}
	if maxWorkers <= 0 {
		maxWorkers = 8
	}
	if maxWorkers > len(keys) {
		maxWorkers = len(keys)
	}

	expiry = resolveExpiry(expiry)

	type job struct {
		idx int
		key string
	}

	jobs := make(chan job, maxWorkers*2)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			item := repomod.PresignItem{Key: j.key}
			if j.key != "" {
				u, err := client.PresignedGetObject(ctx, bucket, j.key, expiry, nil)
				item.Err = err
				if err == nil {
					item.URL = u.String()
					item.ExpiresAt = time.Now().UTC().Add(expiry)
				}
			}
			out[j.idx] = item
		}
	}

	wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

sendLoop:
	for i, k := range keys {
		if k == "" {
			out[i] = repomod.PresignItem{Key: k}
			continue
		}
		select {
		case jobs <- job{idx: i, key: k}:
		case <-ctx.Done():
			break sendLoop
		}
	}
	close(jobs)
	wg.Wait()
	return out
}
