package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func SHA256Hash(data []byte) (string, error) {
	h := sha256.New()
	_, err := h.Write(data)
	if err != nil {
		return "", fmt.Errorf("write data: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// NormalizeAndHash creates a SHA256 hash of a string after normalizing it.
func NormalizeAndHash(input string) string {
	if input == "" {
		return ""
	}
	// Normalize by trimming whitespace, converting to lower case, and compacting multiple spaces.
	normalized := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(input))), " ")
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// Encrypt encrypts the plaintext with the given key using AES-GCM.
func Encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// EncryptString encrypts a string using AES-GCM and returns a base64 encoded string.
func EncryptString(plaintext string) (string, error) {
	key, err := hex.DecodeString(os.Getenv("JOB_ID_ENCRYPTION_KEY"))
	if err != nil {
		return "", fmt.Errorf("cannot decode hex key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

// DecryptString decrypts a base64 encoded string using AES-GCM.
func DecryptString(ciphertext string) (string, error) {
	key, err := hex.DecodeString(os.Getenv("JOB_ID_ENCRYPTION_KEY"))
	if err != nil {
		return "", fmt.Errorf("cannot decode hex key: %w", err)
	}

	data, err := base64.URLEncoding.DecodeString(ciphertext)
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

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, encryptedMessage := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encryptedMessage, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
