// internal/repo/stos3minio/get.go
package stos3minio

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"

	"klynx/config"
)

func GetFileExt(filename string) string {
	return filepath.Ext(filename)
}

func GetS3URL(bucketType string, key string) string {
	if val, ok := config.S3PrivateBuckets[bucketType]; ok {
		return fmt.Sprintf("%s/%s/%s", config.S3BaseURL, val, key)
	}
	if val, ok := config.S3PublicBuckets[bucketType]; ok {
		return fmt.Sprintf("%s/%s/%s", config.S3BaseURL, val, key)
	}
	return fmt.Sprintf("%s/%s", config.S3BaseURL, key)
}

// ถ้าจะเก็บ helper นี้ไว้ ให้รองรับ io.Reader มาตรฐาน
func ReadAll(r io.Reader) ([]byte, error) {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}
