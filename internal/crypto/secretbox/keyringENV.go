// internal/crypto/secretbox/keyringENV.go
package secretbox

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// LoadKeyringFromEnv loads MASTER_KEYRING_JSON from env and converts into *JSONKeyring.
// This implementation does NOT assume JSONKeyring has a field named CurrentKID/CurrentKid.
// It validates using raw JSON keys and then unmarshals into your JSONKeyring struct.
func LoadKeyringFromEnv() (*JSONKeyring, error) {
	raw := strings.TrimSpace(os.Getenv("MASTER_KEYRING_JSON"))
	if raw == "" {
		return nil, fmt.Errorf("MASTER_KEYRING_JSON is empty")
	}

	// 1) validate via raw json to avoid depending on struct field names
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("parse MASTER_KEYRING_JSON: %w", err)
	}

	ck, _ := m["current_kid"].(string)
	if strings.TrimSpace(ck) == "" {
		// some people accidentally use currentKid / currentKID
		if v, ok := m["currentKid"].(string); ok && strings.TrimSpace(v) != "" {
			ck = v
		} else if v, ok := m["currentKID"].(string); ok && strings.TrimSpace(v) != "" {
			ck = v
		}
	}
	if strings.TrimSpace(ck) == "" {
		return nil, fmt.Errorf("invalid keyring: missing current_kid")
	}

	keysAny, ok := m["keys"]
	if !ok {
		return nil, fmt.Errorf("invalid keyring: missing keys")
	}
	keysMap, ok := keysAny.(map[string]any)
	if !ok || len(keysMap) == 0 {
		return nil, fmt.Errorf("invalid keyring: keys is empty")
	}
	if _, ok := keysMap[ck]; !ok {
		return nil, fmt.Errorf("invalid keyring: current_kid %q not found in keys", ck)
	}

	// 2) unmarshal into your real struct (whatever its field names are)
	var kr JSONKeyring
	if err := json.Unmarshal([]byte(raw), &kr); err != nil {
		return nil, fmt.Errorf("unmarshal to JSONKeyring: %w", err)
	}
	return &kr, nil
}
