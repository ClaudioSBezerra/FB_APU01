package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"os"
)

func getServiceEncryptionKey() []byte {
	key := os.Getenv("ENCRYPTION_KEY")
	if key == "" {
		key = os.Getenv("JWT_SECRET")
	}
	if key == "" {
		key = "super-secret-key-change-me-in-prod"
	}
	h := sha256.Sum256([]byte(key))
	return h[:]
}

// DecryptFieldWithFallback decrypts an AES-256-GCM field stored by EncryptField
// in the handlers package. Falls back to returning the raw value if decryption
// fails (handles legacy plaintext rows during migration).
func DecryptFieldWithFallback(encoded string) string {
	key := getServiceEncryptionKey()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return encoded // plaintext fallback
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return encoded
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return encoded
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return encoded
	}
	nonce, ct := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return encoded
	}
	return string(plaintext)
}
