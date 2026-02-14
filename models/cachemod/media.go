// models/cachemod/media.go
package cachemod

import "time"

type MediaStreamCache struct {
	DeviceId      string    `json:"deviceId"`
	StreamKey     string    `json:"streamKey"`
	URLHash       string    `json:"urlHash"`
	VHost         string    `json:"vhost"`
	App           string    `json:"app"`
	IsPublic      bool      `json:"isPublic"`
	CreatedAt     time.Time `json:"createdAt"`
	LastReadyAt   time.Time `json:"lastReadyAt"`
	LastSeenAt    time.Time `json:"lastSeenAt"`
	OriginUrl     string    `json:"originUrl,omitempty"` // optional
	OriginUrlHash string    `json:"originUrlHash,omitempty"`
}
