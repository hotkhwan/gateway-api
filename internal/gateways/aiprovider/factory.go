// internal/gateways/aiprovider/factory.go
package aiprovider

import (
	"fmt"

	"github.com/hotkhwan/gateway-api/internal/crypto/secretbox"
	"github.com/hotkhwan/gateway-api/models/workspacemod"
)

// NewProvider creates the appropriate AIProvider based on the workspace AI config.
// kr is used to decrypt the API key stored in cfg.EncryptedApiKey.
//
// Returns an error if cfg is nil or provider is empty — all providers require an API key.
// Supported provider values: "gemini", "openai", "claude".
func NewProvider(cfg *workspacemod.WorkspaceAIConfig, kr *secretbox.JSONKeyring) (AIProvider, error) {
	if cfg == nil || cfg.Provider == "" {
		return nil, fmt.Errorf("aiprovider: no AI provider configured — please configure a provider and API key in AI Settings")
	}

	apiKey, err := decryptKey(cfg, kr)
	if err != nil {
		return nil, fmt.Errorf("aiprovider: decrypt api key: %w", err)
	}

	switch cfg.Provider {
	case "gemini":
		return NewGeminiProvider(apiKey, cfg.Model), nil
	case "openai":
		return NewOpenAIProvider(apiKey, cfg.Model), nil
	case "claude":
		return NewClaudeProvider(apiKey, cfg.Model), nil
	default:
		return nil, fmt.Errorf("aiprovider: unsupported provider %q", cfg.Provider)
	}
}

// decryptKey decrypts the API key from the workspace AI config.
// Returns an empty string when no key is stored (free-tier / freeSharedProvider mode).
func decryptKey(cfg *workspacemod.WorkspaceAIConfig, kr *secretbox.JSONKeyring) (string, error) {
	if cfg.EncryptedApiKey == nil {
		return "", nil
	}
	if kr == nil {
		return "", fmt.Errorf("keyring is nil but encrypted api key is present")
	}
	return secretbox.DecryptString(kr, *cfg.EncryptedApiKey)
}
