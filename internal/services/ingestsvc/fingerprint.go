// internal/services/ingestsvc/fingerprint.go
package ingestsvc

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

// BuildKeyHash returns the 8-hex SHA256 hash of sorted top-level rawBody keys.
// This is the discriminator used for fingerprint-based template matching (PR5).
func BuildKeyHash(rawBody map[string]any) string {
	keys := make([]string, 0, len(rawBody))
	for k := range rawBody {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	hash := sha256.Sum256([]byte(strings.Join(keys, ",")))
	return fmt.Sprintf("%x", hash)[:8]
}

// BuildFingerprint builds the event fingerprint used for template matching.
//
// Format: vendor|protocol|eventType|subType|schemaVersion|rawBodyKeyHash
//
// vendor/protocol are not known at ingest time — filled empty until template match.
// rawBodyKeyHash = SHA256 of sorted top-level rawBody keys, truncated to 8 hex chars.
func BuildFingerprint(eventType string, rawBody map[string]any) string {
	keyHash := BuildKeyHash(rawBody)
	// vendor|protocol|eventType|subType|schemaVersion|keyHash
	return fmt.Sprintf("||%s|||%s", eventType, keyHash)
}
