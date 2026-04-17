// models/workspacemod/aiConfig.go
package workspacemod

import (
	"time"

	"github.com/hotkhwan/gateway-api/internal/crypto/secretbox"
)

// WorkspaceAIConfig stores AI suggestion settings per workspace.
// Collection: workspace_ai_configs
type WorkspaceAIConfig struct {
	WorkspaceID string `json:"workspaceId" bson:"workspaceId"`
	Enabled     bool   `json:"enabled"     bson:"enabled"`
	// Provider: "gemini" | "openai" | "claude"
	Provider string `json:"provider" bson:"provider"`
	// Model: e.g. "gemini-2.0-flash-lite", "gpt-4o-mini", "claude-haiku-4-5-20251001"
	Model string `json:"model" bson:"model"`
	// EncryptedApiKey — never serialized back to client
	EncryptedApiKey *secretbox.EncBlob `json:"-"              bson:"encryptedApiKey,omitempty"`
	// ProviderMode: "freeSharedProvider" | "workspaceManagedProvider"
	ProviderMode         string     `json:"providerMode"         bson:"providerMode"`
	DefaultTimeoutMs     int        `json:"defaultTimeoutMs"     bson:"defaultTimeoutMs"`
	MaxInputBytes        int        `json:"maxInputBytes"        bson:"maxInputBytes"`
	CreatedBy            string     `json:"createdBy"            bson:"createdBy"`
	UpdatedBy            string     `json:"updatedBy"            bson:"updatedBy"`
	LastValidatedAt      *time.Time `json:"lastValidatedAt"      bson:"lastValidatedAt,omitempty"`
	LastValidationStatus string     `json:"lastValidationStatus" bson:"lastValidationStatus"`
	LastValidationError  string     `json:"lastValidationError"  bson:"lastValidationError"`
	CreatedAt            time.Time  `json:"createdAt"            bson:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"            bson:"updatedAt"`
}
