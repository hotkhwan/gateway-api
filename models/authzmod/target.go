// models/authzmod/target.go
package authzmod

import (
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/x/bsonx/bsoncore"
)

// TargetType constants
const (
	TargetTypeWebhook  = "webhook"
	TargetTypeLine     = "line"
	TargetTypeTelegram = "telegram"
	TargetTypeDiscord  = "discord"
)

// StringSlice is a []string that can decode from either a BSON string or array.
// This provides backward compatibility when migrating a field from string → []string.
type StringSlice []string

func (s StringSlice) MarshalBSONValue() (bsontype.Type, []byte, error) {
	return bson.MarshalValue([]string(s))
}

func (s *StringSlice) UnmarshalBSONValue(t bsontype.Type, data []byte) error {
	switch t {
	case bsontype.String:
		str, _, ok := bsoncore.ReadString(data)
		if !ok {
			return fmt.Errorf("invalid BSON string")
		}
		if str != "" {
			*s = []string{str}
		} else {
			*s = nil
		}
		return nil
	case bsontype.Array:
		var arr []string
		if err := bson.Unmarshal(data, &arr); err != nil {
			// Try raw unmarshal via RawValue
			rv := bson.RawValue{Type: t, Value: data}
			return rv.Unmarshal(&arr)
		}
		*s = arr
		return nil
	case bsontype.Null, bsontype.Undefined:
		*s = nil
		return nil
	default:
		return fmt.Errorf("cannot decode BSON type %s into StringSlice", t)
	}
}

func (s StringSlice) MarshalJSON() ([]byte, error) {
	return json.Marshal([]string(s))
}

func (s *StringSlice) UnmarshalJSON(data []byte) error {
	// Try array first
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*s = arr
		return nil
	}
	// Try single string
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		if str != "" {
			*s = []string{str}
		} else {
			*s = nil
		}
		return nil
	}
	return fmt.Errorf("cannot unmarshal %s into StringSlice", string(data))
}

// TargetConfig เป็น flat union — field ที่ใช้ขึ้นกับ type
type TargetConfig struct {
	// webhook + discord: shared
	URL            string            `bson:"url,omitempty"           json:"url,omitempty"`
	Headers        map[string]string `bson:"headers,omitempty"       json:"headers,omitempty"`
	SigningEnabled bool              `bson:"signingEnabled"          json:"signingEnabled"`
	SigningSecret  string            `bson:"signingSecret,omitempty" json:"signingSecret,omitempty"`
	TimeoutMs      int               `bson:"timeoutMs,omitempty"     json:"timeoutMs,omitempty"`

	// line
	ChannelAccessToken    string      `bson:"channelAccessToken,omitempty"    json:"channelAccessToken,omitempty"`
	ChannelAccessTokenRef string      `bson:"channelAccessTokenRef,omitempty" json:"channelAccessTokenRef,omitempty"`
	To                    StringSlice `bson:"to,omitempty"                    json:"to,omitempty"`

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
