// internal/crypto/secretbox/box.go
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
)

type JSONKeyring struct {
	Current string            `json:"current_kid"`
	Keys    map[string]string `json:"keys"`
}

func NewJSONKeyringFromEnv(raw string) (*JSONKeyring, error) {
	if raw == "" {
		return nil, fmt.Errorf("MASTER_KEYRING_JSON empty")
	}
	var kr JSONKeyring
	if err := json.Unmarshal([]byte(raw), &kr); err != nil {
		return nil, err
	}
	if kr.Current == "" || len(kr.Keys) == 0 {
		return nil, fmt.Errorf("invalid keyring json")
	}
	return &kr, nil
}

func (k *JSONKeyring) currentKey() (kid string, key []byte, err error) {
	kid = k.Current
	s, ok := k.Keys[kid]
	if !ok || s == "" {
		return "", nil, fmt.Errorf("kid not found: %s", kid)
	}
	key, err = base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", nil, fmt.Errorf("kid %s bad base64: %w", kid, err)
	}
	if len(key) != 32 {
		return "", nil, fmt.Errorf("kid %s must be 32 bytes, got %d", kid, len(key))
	}
	return kid, key, nil
}

func (k *JSONKeyring) keyByID(kid string) ([]byte, error) {
	s, ok := k.Keys[kid]
	if !ok || s == "" {
		return nil, fmt.Errorf("kid not found: %s", kid)
	}
	key, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("kid %s bad base64: %w", kid, err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("kid %s must be 32 bytes, got %d", kid, len(key))
	}
	return key, nil
}

// ----- Encrypted blob -----

type EncBlob struct {
	V     int    `bson:"v" json:"v"`
	Alg   string `bson:"alg" json:"alg"`
	KID   string `bson:"kid" json:"kid"`
	Nonce string `bson:"nonce" json:"nonce"`
	CT    string `bson:"ct" json:"ct"`
}

func EncryptString(kr *JSONKeyring, plaintext string) (*EncBlob, error) {
	kid, key, err := kr.currentKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return &EncBlob{
		V:     1,
		Alg:   "AES-256-GCM",
		KID:   kid,
		Nonce: base64.StdEncoding.EncodeToString(nonce),
		CT:    base64.StdEncoding.EncodeToString(ct),
	}, nil
}

func DecryptString(kr *JSONKeyring, blob EncBlob) (string, error) {
	if blob.Alg != "AES-256-GCM" || blob.V != 1 {
		return "", fmt.Errorf("unsupported enc blob v=%d alg=%s", blob.V, blob.Alg)
	}
	key, err := kr.keyByID(blob.KID)
	if err != nil {
		return "", err
	}
	nonce, err := base64.StdEncoding.DecodeString(blob.Nonce)
	if err != nil {
		return "", err
	}
	ct, err := base64.StdEncoding.DecodeString(blob.CT)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
