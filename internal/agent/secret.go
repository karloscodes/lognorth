// Package agent holds the autonomous issue-fixing feature: encrypted credentials,
// GitHub validation, the fix queue, and the worker that drives opencode.
package agent

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// deriveKey turns the instance private key (LOGNORTH_PRIVATE_KEY) into a 32-byte
// AES key. The same key always derives the same AES key, so values stay readable
// across restarts.
func deriveKey(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// Encrypt encrypts plaintext with AES-256-GCM and returns base64(nonce|ciphertext).
// Empty plaintext returns "" — there's nothing to store.
func Encrypt(plaintext, secret string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	gcm, err := newGCM(secret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

// Decrypt reverses Encrypt. Empty input returns "". A wrong key or tampered
// ciphertext returns an error (GCM authentication fails).
func Decrypt(encoded, secret string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(secret)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func newGCM(secret string) (cipher.AEAD, error) {
	block, err := aes.NewCipher(deriveKey(secret))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
