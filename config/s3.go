// config/s3.go
package config

import (
	"context"
	"fmt"
	"os"
	"strings"

	"klynx/internal/logger"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var (
	S3Client         *minio.Client
	S3BaseURL        string
	S3PublicBuckets  map[string]string
	S3PrivateBuckets map[string]string
)

// ✅ Helper to get env with fallback
func getEnvOrDefault(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}

// ✅ Initialize S3 and ensure buckets exist
func InitS3() {
	log := logger.Boot("S3", "config-InitS3")
	S3BaseURL = getEnvOrDefault("S3_BASE_URL", "https://aliza-s3.k-lynx.com")

	endpoint := getEnvOrDefault("S3_ENDPOINT", "minio.klynx.local:9000")
	accessKey := getEnvOrDefault("S3_ACCESS_KEY", "minioadmin")
	secretKey := getEnvOrDefault("S3_SECRET_KEY", "minioadmin")

	S3PublicBuckets = make(map[string]string)
	S3PrivateBuckets = make(map[string]string)

	// 🔹 โหลด bucket list จาก ENV (คั่นด้วย ,)
	publicList := strings.Split(getEnvOrDefault("S3_PUBBUCKET", "public"), ",")
	for _, b := range publicList {
		b = strings.TrimSpace(b)
		if b != "" {
			S3PublicBuckets[b] = b
		}
	}
	privateList := strings.Split(getEnvOrDefault("S3_PRIBUCKET", "analytic,kwatch,document"), ",")
	for _, b := range privateList {
		b = strings.TrimSpace(b)
		if b != "" {
			S3PrivateBuckets[b] = b
		}
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: true,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("❌ Failed to init S3 client")
	}
	S3Client = client
	log.Info().Str("endpoint", endpoint).Msg("✅ S3 client initialized")

	ctx := context.Background()

	// ✅ loop ทั้ง public และ private
	for name, bucket := range S3PublicBuckets {
		ensureBucket(ctx, bucket, true)
		log.Info().Msgf("🔓 Public bucket %s ready", name)
	}
	for name, bucket := range S3PrivateBuckets {
		ensureBucket(ctx, bucket, false)
		log.Info().Msgf("🔒 Private bucket %s ready", name)
	}
}

func ensureBucket(ctx context.Context, bucket string, isPublic bool) {
	log := logger.Boot("S3", "config-ensureBucket")

	exists, err := S3Client.BucketExists(ctx, bucket)
	if err != nil {
		log.Fatal().Err(err).Str("bucket", bucket).Msg("❌ Failed to check bucket")
	}
	if !exists {
		if err := S3Client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			log.Fatal().Err(err).Str("bucket", bucket).Msg("❌ Failed to create bucket")
		}
		log.Info().Str("bucket", bucket).Msg("✅ S3 bucket created")
	}

	if isPublic {
		publicPolicy := fmt.Sprintf(`{
			"Version": "2012-10-17",
			"Statement": [{
				"Effect": "Allow",
				"Principal": {"AWS": ["*"]},
				"Action": ["s3:GetObject"],
				"Resource": ["arn:aws:s3:::%s/*"]
			}]
		}`, bucket)

		if err := S3Client.SetBucketPolicy(ctx, bucket, publicPolicy); err != nil {
			log.Error().Err(err).Str("bucket", bucket).Msg("⚠️ Failed to set public policy")
		} else {
			log.Info().Str("bucket", bucket).Msg("✅ Public read policy applied")
		}
	}
}

func S3PutOptionsContentZip() minio.PutObjectOptions {
	return minio.PutObjectOptions{ContentType: "application/zip"}
}
