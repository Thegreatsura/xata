// Package secretcipher encrypts secrets at rest with AES-256-GCM under a
// server-side key.
package secretcipher

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

// versionV1 is the current ciphertext version. Stored values carry it as a
// prefix so the key can be rotated later without re-encrypting everything:
// introduce v2 as the write path and keep v1 on the read path until the old
// rows are gone. Retrofitting this onto existing ciphertexts is painful, so it
// is written from the start even though only one key exists today.
const versionV1 = "v1"

const (
	// KeyLen is the AES-256 key size.
	KeyLen = 32
	// nonceLen is the standard GCM nonce size.
	nonceLen = 12
)

// Cipher encrypts and decrypts secrets using AES-256-GCM. Safe for concurrent
// use.
type Cipher struct {
	aead cipher.AEAD
}

// New builds a Cipher from a base64-encoded 32-byte key.
func New(base64Key string) (*Cipher, error) {
	if base64Key == "" {
		return nil, fmt.Errorf("encryption key is required")
	}
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("decode encryption key: %w", err)
	}
	if len(key) != KeyLen {
		return nil, fmt.Errorf("encryption key must be %d bytes, got %d", KeyLen, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns the plaintext encrypted for storage, as
// "v1:base64(nonce||ciphertext)". A fresh nonce is drawn for every call, so the
// same plaintext does not encrypt to the same value twice. The same
// associatedData must be passed to Decrypt; it binds the ciphertext to its row.
func (c *Cipher) Encrypt(plaintext string, associatedData []byte) (string, error) {
	if plaintext == "" {
		return "", fmt.Errorf("plaintext is empty")
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	// Seal appends the ciphertext to the nonce, giving nonce||ciphertext.
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), associatedData)
	return versionV1 + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. It fails if the value was tampered with, encrypted
// under a different key or associated data, or carries an unknown version.
func (c *Cipher) Decrypt(stored string, associatedData []byte) (string, error) {
	version, payload, found := strings.Cut(stored, ":")
	if !found {
		return "", fmt.Errorf("malformed ciphertext: no version prefix")
	}
	if version != versionV1 {
		return "", fmt.Errorf("unsupported ciphertext version %q", version)
	}

	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	if len(raw) < nonceLen {
		return "", fmt.Errorf("malformed ciphertext: shorter than nonce")
	}

	nonce, sealed := raw[:nonceLen], raw[nonceLen:]
	plaintext, err := c.aead.Open(nil, nonce, sealed, associatedData)
	if err != nil {
		// Deliberately not wrapping err: GCM failures are indistinguishable by
		// design and the message must not hint at the key or plaintext.
		return "", fmt.Errorf("decrypt secret")
	}
	return string(plaintext), nil
}
