// models/kwatmod/crimes.go
package kwatmod

import "go.mongodb.org/mongo-driver/bson/primitive"

// ใช้ตอน cleanup ก่อน DeleteMany เพื่อรู้อะไรต้องลบทิ้งบ้าง
type CrimesDocMinimal struct {
	PersonKey      string      `bson:"personKey"`
	IDCard         string      `bson:"idcard,omitempty"`
	PhotoOriginKey string      `bson:"photoOriginKey,omitempty"`
	PhotoFaceKey   string      `bson:"photoFaceKey,omitempty"`
	PhotoKeyLegacy string      `bson:"photoKey,omitempty"` // schema เก่า
	External       ExternalAll `bson:"external,omitempty"`
}

type WatchmanAPIRespError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  bool   `json:"status"`
}

type WlMini struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"`
	PersonKey      string             `bson:"personKey"`
	IDCard         string             `bson:"idcard"`
	PhotoKey       string             `bson:"photoKey"`
	PhotoOriginKey string             `bson:"photoOriginKey"`
	PhotoFaceKey   string             `bson:"photoFaceKey"`
	External       struct {
		IBOC struct {
			ID     string `bson:"id"`
			FaceID string `bson:"faceId"`
		} `bson:"iboc"`
		IBOCDev struct {
			ID     string `bson:"id"`
			FaceID string `bson:"faceId"`
		} `bson:"ibocdev"`
		Watchman struct {
			ID     string `bson:"id"`
			IDCard string `bson:"idCard"`
		} `bson:"watchman"`
	} `bson:"external"`
}
