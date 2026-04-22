// internal/services/licensesvc/key.go
package licensesvc

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ErrSecretRequired is returned when the HMAC secret is missing. License keys
// must be signable so the platform can prove which deployment issued them.
var ErrSecretRequired = errors.New("license signing secret is required")

// KeyFormat documents the visible shape of an issued license key.
const KeyFormat = "XXXX-YYYY-ZZZZ-WWWW"

// Generate produces a new license key in the canonical XXXX-YYYY-ZZZZ-WWWW
// format. The returned key embeds 8 hex chars of random entropy plus 8 hex
// chars of HMAC-SHA256 signature derived from the full 16-byte random payload.
//
// Extracted from the previous cmd/license.go CLI so the same key-minting
// logic is reachable from both the HTTP admin surface and any remaining
// backfill paths. Authoritative validation happens via the repo — the
// signature suffix is a cheap issuer tag, not a self-verifying artifact.
func Generate(secret string) (string, error) {
	if secret == "" {
		return "", ErrSecretRequired
	}

	entropy := make([]byte, 16)
	if _, err := rand.Read(entropy); err != nil {
		return "", fmt.Errorf("license entropy: %w", err)
	}

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(entropy)
	signature := hex.EncodeToString(h.Sum(nil))

	combined := hex.EncodeToString(entropy)[:8] + signature[:8]

	segments := []string{
		combined[0:4],
		combined[4:8],
		combined[8:12],
		combined[12:16],
	}
	return strings.ToUpper(strings.Join(segments, "-")), nil
}

// ValidateFormat returns true when key matches XXXX-YYYY-ZZZZ-WWWW with
// uppercase hex segments. It is a shape check only — the authoritative
// validity of a license lives in the license_keys collection.
func ValidateFormat(key string) bool {
	parts := strings.Split(key, "-")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if len(part) != 4 {
			return false
		}
		for _, c := range part {
			if !((c >= 'A' && c <= 'F') || (c >= '0' && c <= '9')) {
				return false
			}
		}
	}
	return true
}
