// utils/basePath.go
package utils

import (
	"os"
	"strings"
)

func GetBasePath() string {
	base := strings.TrimSpace(os.Getenv("BASE_PATH"))
	if base == "" {
		base = "/api/v1" // default
	}
	// remove trailing slash เช่น "/api/v1/"
	return strings.TrimRight(base, "/")
}

// NEW: get files proxy path from env
func GetFilesProxyPath() string {
	p := strings.TrimSpace(os.Getenv("FILES_PROXY_PATH"))
	if p == "" {
		p = "/files" // default
	}
	return "/" + strings.Trim(p, "/") // normalize → ไม่มี double slash
}
