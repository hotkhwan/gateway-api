// models/authzmod/target.go
package authzmod

import "time"

// TargetType constants
const (
	TargetTypeWebhook  = "webhook"
	TargetTypeLine     = "line"
	TargetTypeTelegram = "telegram"
	TargetTypeDiscord  = "discord"
)

// TargetConfig เป็น flat union — field ที่ใช้ขึ้นกับ type
type TargetConfig struct {
	// webhook + discord: shared
	URL            string            `bson:"url,omitempty"           json:"url,omitempty"`
	Headers        map[string]string `bson:"headers,omitempty"       json:"headers,omitempty"`
	SigningEnabled bool              `bson:"signingEnabled"          json:"signingEnabled"`
	SigningSecret  string            `bson:"signingSecret,omitempty" json:"signingSecret,omitempty"`
	TimeoutMs      int               `bson:"timeoutMs,omitempty"     json:"timeoutMs,omitempty"`

	// line
	ChannelAccessToken string `bson:"channelAccessToken,omitempty" json:"channelAccessToken,omitempty"`
	To                 string `bson:"to,omitempty"                 json:"to,omitempty"`

	// telegram
	BotToken string `bson:"botToken,omitempty" json:"botToken,omitempty"`
	ChatId   string `bson:"chatId,omitempty"   json:"chatId,omitempty"`
}

// DeliveryTarget คือ "ปลายทาง" ที่ event จะถูก forward ไป
type DeliveryTarget struct {
	TargetId string `bson:"targetId" json:"id"`
	TenantId string `bson:"tenantId" json:"-"`
	OrgId    string `bson:"orgId"    json:"orgId"`

	Name    string `bson:"name"    json:"name"`
	Type    string `bson:"type"    json:"type"`    // webhook|line|telegram|discord
	Enabled bool   `bson:"enabled" json:"enabled"` // default true

	Config TargetConfig `bson:"config" json:"config"`

	CreatedBy string    `bson:"createdBy" json:"createdBy"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}
