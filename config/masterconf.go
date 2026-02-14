// config/masterconf.go
package config

import (
	"os"
	"sync"

	"klynx/internal/crypto/secretbox"
)

var (
	keyringOnce sync.Once
	keyring     *secretbox.JSONKeyring
	keyringErr  error
)

func MasterKeyring() (*secretbox.JSONKeyring, error) {
	keyringOnce.Do(func() {
		keyring, keyringErr = secretbox.NewJSONKeyringFromEnv(os.Getenv("MASTER_KEYRING_JSON"))
	})
	return keyring, keyringErr
}
