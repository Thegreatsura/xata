package key

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"hash/crc32"
	"strings"

	"github.com/jxskiss/base62"
)

// This implementation is highly inspired by
// https://github.blog/2021-04-05-behind-githubs-new-authentication-token-formats/
const (
	// Number of random bytes used to generate the key
	lengthBytes = 20

	// Max length for API keys == prefix_ + base62(lengthBytes bytes + 4 crc)
	MaxLength = 40

	// Prefix used for user keys
	UserKeyPrefix = "xau"

	// Prefix used for organization keys
	OrganizationKeyPrefix = "xao"

	// Character used to obfuscate keys
	obfuscatedChar = "*"

	// Default number of characters to show at the end of an obfuscated key
	DefaultObfuscateCharsCount = 3
)

// API key to be used
type Key string

var encoder = base62.NewEncoding("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// NewUserKey generates and returns a new user API key
func NewUserKey() (Key, error) {
	return newKey(UserKeyPrefix)
}

// NewOrganizationKey generates and returns a new organization API key
func NewOrganizationKey() (Key, error) {
	return newKey(OrganizationKeyPrefix)
}

func newKey(prefix string) (Key, error) {
	b := make([]byte, lengthBytes, lengthBytes+4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b = append(b, i32tob(crc32.ChecksumIEEE(b))...)

	return Key(prefix + "_" + encoder.EncodeToString(b)), nil
}

func (k Key) String() string {
	return string(k)
}

func (k Key) IsValid() bool {
	if len(k) > MaxLength {
		return false
	}

	parts := strings.Split(string(k), "_")
	if len(parts) != 2 {
		return false
	}

	// check prefix
	if !strings.HasPrefix(parts[0], UserKeyPrefix) &&
		!strings.HasPrefix(parts[0], OrganizationKeyPrefix) {
		return false
	}

	// check key part
	decoded, err := encoder.DecodeString(parts[1])
	if err != nil {
		return false
	}

	if len(decoded) < 4 {
		return false
	}

	key := decoded[:len(decoded)-4]
	checksum := btoi32(decoded[len(decoded)-4:])

	// key is valid if checksum is
	if crc32.ChecksumIEEE(key) != checksum {
		return false
	}

	return true
}

// Obfuscate will return a string that preserves the prefix (everything before and including the underscore '_')
// and the last N characters of the key, replacing all characters in between with '*'.
// If the key is shorter than N, the key is returned without obfuscating.
func (k Key) Obfuscate(showLastNChars int) string {
	if showLastNChars < 0 {
		showLastNChars = 0
	}
	keyStr := k.String()
	if len(keyStr) <= showLastNChars {
		return keyStr
	}

	// Find the prefix (everything up to and including the underscore)
	prefixEndIndex := strings.Index(keyStr, "_")
	if prefixEndIndex == -1 {
		// If no underscore found, use the standard obfuscation logic
		obfuscatedSecret := bytes.NewBufferString(strings.Repeat(obfuscatedChar, len(keyStr)-showLastNChars))
		obfuscatedSecret.WriteString(keyStr[len(keyStr)-showLastNChars:])
		return obfuscatedSecret.String()
	}

	prefixEndIndex++ // Include the underscore
	prefix := keyStr[:prefixEndIndex]

	// Calculate how many characters to obfuscate in the middle
	endIndex := len(keyStr) - showLastNChars
	middleLength := endIndex - prefixEndIndex

	// Handle edge case where prefix plus shown chars would overlap
	if middleLength < 0 {
		// In this case, show the entire key
		return keyStr
	}

	// Build the obfuscated string: prefix + asterisks + last N chars
	var obfuscatedSecret bytes.Buffer
	obfuscatedSecret.WriteString(prefix)
	obfuscatedSecret.WriteString(strings.Repeat(obfuscatedChar, middleLength))
	obfuscatedSecret.WriteString(keyStr[endIndex:])

	return obfuscatedSecret.String()
}

// ExtractUnobfuscatedPart returns the unobfuscated part of the key, which now includes
// both the prefix at the start and any non-asterisk characters at the end.
func (k Key) extractUnobfuscatedPart() string {
	keyStr := k.String()

	// If no asterisks, return the full key
	if !strings.Contains(keyStr, obfuscatedChar) {
		return keyStr
	}

	var result bytes.Buffer

	// Extract all non-obfuscated characters
	for _, char := range keyStr {
		if string(char) != obfuscatedChar {
			result.WriteRune(char)
		}
	}

	return result.String()
}

// Matches will check if two keys are a match. If they are the same value, or if
// the unobfuscated part of one key matches the corresponding parts of the other key,
// it will return true. Otherwise, it will return false.
func (k Key) Matches(otherKey Key) bool {
	// exact match
	if k == otherKey {
		return true
	}

	// Check if at least one of the keys has obfuscated characters
	keyStr := k.String()
	otherKeyStr := otherKey.String()

	if !strings.Contains(keyStr, obfuscatedChar) && !strings.Contains(otherKeyStr, obfuscatedChar) {
		return false
	}

	// If this key is not obfuscated (full key) and other key is obfuscated
	if !strings.Contains(keyStr, obfuscatedChar) && strings.Contains(otherKeyStr, obfuscatedChar) {
		// Get prefix of obfuscated key (if it has one)
		prefixEndIndex := strings.Index(otherKeyStr, "_")
		if prefixEndIndex != -1 {
			prefixEndIndex++ // Include the underscore
			prefix := otherKeyStr[:prefixEndIndex]

			// Check that the prefix matches
			if !strings.HasPrefix(keyStr, prefix) {
				return false
			}

			// Get suffix (everything after the last asterisk)
			lastAsteriskIndex := strings.LastIndex(otherKeyStr, obfuscatedChar)
			if lastAsteriskIndex < len(otherKeyStr)-1 {
				suffix := otherKeyStr[lastAsteriskIndex+1:]
				return strings.HasSuffix(keyStr, suffix)
			}

			// If there's no suffix, any key with the matching prefix could match
			return true
		}

		// If no prefix in obfuscated key, check if the non-obfuscated ending matches
		lastAsteriskIndex := strings.LastIndex(otherKeyStr, obfuscatedChar)
		if lastAsteriskIndex < len(otherKeyStr)-1 {
			suffix := otherKeyStr[lastAsteriskIndex+1:]
			return strings.HasSuffix(keyStr, suffix)
		}

		return false
	}

	// If other key is not obfuscated (full key) and this key is obfuscated
	if !strings.Contains(otherKeyStr, obfuscatedChar) && strings.Contains(keyStr, obfuscatedChar) {
		// Apply the same logic, but with keys reversed
		return otherKey.Matches(k)
	}

	// Both keys are partially obfuscated
	// Extract unobfuscated parts and check if they match
	kUnobfuscated := k.extractUnobfuscatedPart()
	otherUnobfuscated := otherKey.extractUnobfuscatedPart()
	return kUnobfuscated == otherUnobfuscated
}

func i32tob(val uint32) []byte {
	r := make([]byte, 4)
	for i := range uint32(4) {
		r[i] = byte((val >> (8 * i)) & 0xff)
	}
	return r
}

func btoi32(val []byte) uint32 {
	r := uint32(0)
	for i := range uint32(4) {
		r |= uint32(val[i]) << (8 * i)
	}
	return r
}

// HashKey generates an HMAC-SHA256 of the key for fast lookups
func (k Key) HashKey(secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(k.String()))
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateHash compares the key with a hash and returns an error if they don't match
func (k Key) ValidateHash(hash string, secret string) bool {
	expectedHMAC := k.HashKey(secret)
	return hmac.Equal([]byte(expectedHMAC), []byte(hash))
}
