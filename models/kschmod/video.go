// models/kschmod/video.go
package kschmod

import (
	"io"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type VideoDoc struct {
	ID               primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name             string             `bson:"name" json:"name"`
	Description      string             `bson:"description,omitempty" json:"description,omitempty"`
	Status           bool               `bson:"status" json:"status"`
	State            string             `bson:"state" json:"state"`
	VideoKey         string             `bson:"videoKey" json:"videoKey"`
	VideoContentType string             `bson:"videoContentType" json:"videoContentType"`
	VideoSize        int64              `bson:"videoSize" json:"videoSize"`
	CreatedAt        time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt        time.Time          `bson:"updatedAt" json:"updatedAt"`
}

type VideoCreateRequest struct {
	ID          primitive.ObjectID `json:"-"`
	Name        string             `form:"name" validate:"required"`
	Description string             `form:"description"`
	Status      string             `form:"status"` // "true"/"false"
	State       string             `form:"state"`
	// ไฟล์ (streaming; ไม่โหลดทั้งไฟล์ขึ้น RAM)
	VideoR           io.Reader `json:"-"`
	VideoSize        int64     `json:"-"`
	VideoFileName    string    `json:"-"`
	VideoContentType string    `json:"-"`
	VideoPath        string    `json:"-"`
	NoSkip           bool      `json:"-"`
	SkipFrame        int64     `json:"-"`
}

// Kafka Event
type VideoEvent struct {
	ID               string `json:"id"`
	Event            string `json:"event"` // "video.created" | "video.updated" | "video.deleted"
	Time             string `json:"time"`  // RFC3339Nano
	Name             string `json:"name"`
	Status           bool   `json:"status"`
	State            string `json:"state"`
	VideoKey         string `json:"videoKey"`
	VideoContentType string `json:"videoContentType"`
	VideoSize        int64  `json:"videoSize"`
	VideoPath        string `json:"videoPath"`
	NoSkip           bool   `json:"noSkip"`
	SkipFrame        int    `json:"skipFrame"`
	Rev              int64  `json:"rev,omitempty"` // ✅ เพิ่มตรงนี้
}

type VideoResponse struct {
	ID               string    `bson:"_id,omitempty" json:"id"`
	Name             string    `bson:"name" json:"name"`
	Description      string    `bson:"description,omitempty" json:"description,omitempty"`
	Status           bool      `bson:"status" json:"status"`
	State            string    `bson:"state" json:"state"`
	VideoKey         string    `bson:"videoKey" json:"videoKey"`
	VideoContentType string    `bson:"videoContentType" json:"videoContentType"`
	VideoSize        int64     `bson:"videoSize" json:"videoSize"`
	CreatedAt        time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt        time.Time `bson:"updatedAt" json:"updatedAt"`
	VideoURL         string    `json:"video_url,omitempty"`
}
