// models/repomod/stos3miniomod.go
package repomod

import (
	"context"
	"net/url"
	"time"
)

type Presigner interface {
	PresignedGetObject(ctx context.Context, bucketName, objectName string, expires time.Duration, reqParams url.Values) (*url.URL, error)
}

type PresignItem struct {
	Key       string    `json:"key"`
	URL       string    `json:"url,omitempty"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
	Err       error     `json:"-"`               // ไม่ serialize ตรง ๆ
	ErrString string    `json:"error,omitempty"` // จะเซ็ตจาก svc ถ้ามี
}
