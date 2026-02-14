// utils/cryptoutil.go
package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type encryptedPayload struct {
	V     int    `json:"v" bson:"v"`
	Alg   string `json:"alg" bson:"alg"`
	KID   string `json:"kid" bson:"kid"`
	Nonce string `json:"nonce" bson:"nonce"`
	CT    string `json:"ct" bson:"ct"`
}

type keyringV2 struct {
	CurrentKID string            `json:"current_kid"`
	Keys       map[string]string `json:"keys"`
}

var (
	ErrKeyringNotSet   = errors.New("MASTER_KEYRING_JSON not set")
	ErrKeyNotFound     = errors.New("key id not found in keyring")
	ErrUnsupportedAlgo = errors.New("unsupported encryption algorithm")
)

func decodeB64Any(s string) ([]byte, error) {
	decoders := []struct {
		name string
		dec  *base64.Encoding
	}{
		{"std", base64.StdEncoding},
		{"rawStd", base64.RawStdEncoding},
		{"url", base64.URLEncoding},
		{"rawUrl", base64.RawURLEncoding},
	}

	var lastErr error
	for _, d := range decoders {
		b, err := d.dec.DecodeString(s)
		if err == nil {
			return b, nil
		}
		lastErr = fmt.Errorf("%s decode: %w", d.name, err)
	}
	return nil, lastErr
}

func DecryptFromKeyringJSON(enc map[string]any) (string, error) {
	raw, err := json.Marshal(enc)
	if err != nil {
		return "", err
	}

	var payload encryptedPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}

	if payload.Alg != "AES-256-GCM" {
		return "", ErrUnsupportedAlgo
	}

	keyringJSON := os.Getenv("MASTER_KEYRING_JSON")
	if keyringJSON == "" {
		return "", ErrKeyringNotSet
	}

	var kr keyringV2
	if err := json.Unmarshal([]byte(keyringJSON), &kr); err != nil {
		return "", fmt.Errorf("invalid MASTER_KEYRING_JSON: %w", err)
	}

	keyB64, ok := kr.Keys[payload.KID]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrKeyNotFound, payload.KID)
	}

	key, err := decodeB64Any(keyB64)
	if err != nil {
		return "", fmt.Errorf("decode key: %w", err)
	}

	nonce, err := decodeB64Any(payload.Nonce)
	if err != nil {
		return "", fmt.Errorf("decode nonce: %w", err)
	}

	ciphertext, err := decodeB64Any(payload.CT)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt failed: %w", err)
	}

	return string(plain), nil
}
