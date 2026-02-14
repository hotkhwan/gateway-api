// internal/repo/stos3minio/util.go
package stos3minio

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"klynx/config"
)

// parseS3URL รองรับ:
// - s3://bucket/key
// - https://endpoint/bucket/key             (path-style)
// - https://bucket.endpoint/key             (virtual-hosted)
// - https://.../bucket/key?X-Amz-...       (presigned -> ตัด query)
func parseS3URL(fullURL string) (bucket, objectKey string, err error) {
	u, e := url.Parse(fullURL)
	if e != nil {
		return "", "", fmt.Errorf("invalid URL: %w", e)
	}

	// s3://
	if u.Scheme == "s3" {
		if u.Host == "" || u.Path == "" {
			return "", "", fmt.Errorf("invalid s3 url: %s", fullURL)
		}
		return u.Host, strings.TrimPrefix(u.Path, "/"), nil
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", fmt.Errorf("invalid S3 URL (scheme): %s", fullURL)
	}

	// ตัด query/fragment
	path := u.EscapedPath()
	path = strings.TrimPrefix(path, "/")

	// หา endpoint (จาก S3BaseURL > S3_ENDPOINT > URL เอง)
	endpointHost := hostFrom(config.S3BaseURL)
	if endpointHost == "" {
		endpointHost = hostFrom(os.Getenv("S3_ENDPOINT"))
	}
	if endpointHost == "" {
		endpointHost = u.Hostname()
	}

	host := u.Hostname()

	// virtual-hosted: <bucket>.<endpoint>
	if host != endpointHost && strings.HasSuffix(host, "."+endpointHost) {
		bucket = strings.TrimSuffix(host, "."+endpointHost)
		if bucket == "" {
			return "", "", fmt.Errorf("invalid virtual-hosted host: %s", host)
		}
		key, _ := url.PathUnescape(path)
		if key == "" {
			return "", "", fmt.Errorf("empty key in URL: %s", fullURL)
		}
		return bucket, key, nil
	}

	// path-style: /<bucket>/<key...>
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid S3 path: %s", path)
	}
	key, _ := url.PathUnescape(parts[1])
	return parts[0], key, nil
}

func hostFrom(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		if u, err := url.Parse(raw); err == nil {
			return u.Hostname()
		}
	}
	return raw
}

// LooksLikeURL: ช่วยเช็คว่าเป็น URL ที่ควร parse ไหม
func LooksLikeURL(s string) bool {
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "s3://")
}
