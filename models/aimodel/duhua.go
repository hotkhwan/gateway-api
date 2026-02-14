package aimodel

import "time"

type DuhuaMessage struct {
	ID    string `json:"id"`
	Event string `json:"event"`
	Time  string `json:"time"`
	State string `json:"state,omitempty"`
	Rev   int64  `json:"rev,omitempty"`

	DeviceID  string    `json:"deviceId"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	ImgPath1  string    `json:"imgPath1"`
	ImgPath2  string    `json:"imgPath2,omitempty"`
	ImgPath3  string    `json:"imgPath3"`
	ImgPath4  string    `json:"imgPath4,omitempty"`

	// ✅ New fields
	Action          string                 `json:"action,omitempty"`
	Code            string                 `json:"code,omitempty"`
	FaceAttributes  map[string]interface{} `json:"faceAttributes,omitempty"`
	HumanAttributes map[string]interface{} `json:"humanAttributes,omitempty"`
}
