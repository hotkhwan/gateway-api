// models/devsyncmod/atamod.go
package devsyncmod

import "go.mongodb.org/mongo-driver/bson/primitive"

type ATASyncResponse struct {
	Code    string     `json:"code" example:"ATA_SYNC_OK"`
	Message string     `json:"message" example:"ATA devices/channels synced"`
	Status  bool       `json:"status" example:"true"`
	Detail  SyncResult `json:"detail"`
}

type ATAEdgeDoc struct {
	ID           primitive.ObjectID `bson:"_id"`
	Type         string             `bson:"type"`
	Username     string             `bson:"username"`
	Name         string             `bson:"name,omitempty"`
	URL          string             `bson:"url"`
	TLS          bool               `bson:"tls"`
	PassEnc      map[string]any     `bson:"passEnc"`
	ApiKey       string             `bson:"apiKey,omitempty"`       // ✅ string ตาม mongo
	ApiSecretEnc map[string]any     `bson:"apiSecretEnc,omitempty"` // ไม่ decode ก็รับไว้เฉยๆ
}
