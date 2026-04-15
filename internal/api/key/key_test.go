package key

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeyValidation(t *testing.T) {
	userKey, err := NewUserKey()
	assert.NoError(t, err)

	orgKey, err := NewOrganizationKey()
	assert.NoError(t, err)

	tests := []struct {
		name  string
		key   Key
		valid bool
	}{
		{
			name:  "generated user key",
			key:   userKey,
			valid: true,
		},
		{
			name:  "generated organization key",
			key:   orgKey,
			valid: true,
		},
		{
			name:  "valid known key",
			key:   Key("xau_xoWnROvXetYNTeVzfIyA10QcC8UNZufs2"),
			valid: true,
		},
		{
			name:  "valid known key",
			key:   Key("xao_xoWnROvXetYNTeVzfIyA10QcC8UNZufs2"),
			valid: true,
		},
		{
			name:  "no underscore",
			key:   Key("xauxoWnROvXetYNTeVzfIyA10QcC8UNZufs2"),
			valid: false,
		},
		{
			name:  "invalid prefix",
			key:   Key("foo_xoWnROvXetYNTeVzfIyA10QcC8UNZufs2"),
			valid: false,
		},
		{
			name:  "changed key part",
			key:   Key("xau_foWnROvXetYNTeVzfIyA10QcC8UNZufs2"),
			valid: false,
		},
		{
			name:  "really short key",
			key:   Key("xau_234s"),
			valid: false,
		},
		{
			name:  "too big key",
			key:   Key("xau_foWnROvXetYNTeVzfIyA10QcC8UNZufs2yz3x"),
			valid: false,
		},
		{
			name:  "not base62",
			key:   Key("xau_foWnROvXetYNTeVzfIyA10QcC8!%ufs!"),
			valid: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Log(test.key)
			assert.Equal(t, test.valid, test.key.IsValid())
		})
	}
}

func TestKeyObfuscate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		key             Key
		showNLastNChars int
		wantSecret      string
	}{
		{
			name:            "user key with prefix preserved",
			key:             "xau_abc123defGHIjklMNOpqrSTV",
			showNLastNChars: 4,
			wantSecret:      "xau_********************rSTV",
		},
		{
			name:            "organization key with prefix preserved",
			key:             "xao_xyz789ABCDEF12345",
			showNLastNChars: 3,
			wantSecret:      "xao_**************345",
		},
		{
			name:            "no underscore in key",
			key:             "testing123456",
			showNLastNChars: 4,
			wantSecret:      "*********3456",
		},
		{
			name:            "prefix longer than key minus shown chars",
			key:             "xau_12345",
			showNLastNChars: 3,
			wantSecret:      "xau_**345",
		},
		{
			name:            "prefix plus shown chars longer than key",
			key:             "xau_123",
			showNLastNChars: 4,
			wantSecret:      "xau_123",
		},
		{
			name:            "negative number of characters",
			key:             "xau_abc123",
			showNLastNChars: -5,
			wantSecret:      "xau_******",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			secret := tc.key.Obfuscate(tc.showNLastNChars)
			assert.Equal(t, tc.wantSecret, secret)
		})
	}
}

func TestKeyGeneration(t *testing.T) {
	t.Parallel()

	userKey, err := NewUserKey()
	assert.NoError(t, err)

	orgKey, err := NewOrganizationKey()
	assert.NoError(t, err)

	tests := []struct {
		name      string
		existing  Key
		other     Key
		wantMatch bool
	}{
		{
			name:      "match user key with prefix preserved obfuscation",
			existing:  userKey,
			other:     Key(userKey.Obfuscate(6)),
			wantMatch: true,
		},
		{
			name:      "match org key with prefix preserved obfuscation",
			existing:  orgKey,
			other:     Key(orgKey.Obfuscate(8)),
			wantMatch: true,
		},
		{
			name:      "no match between user and org keys with prefix preserved",
			existing:  userKey,
			other:     Key(orgKey.Obfuscate(6)),
			wantMatch: false,
		},
		{
			name:      "custom key with no prefix should still work with obfuscation",
			existing:  Key("custom_key_with_no_standard_prefix"),
			other:     Key("custom_*********************efix"),
			wantMatch: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			match := tc.existing.Matches(tc.other)
			assert.Equal(t, tc.wantMatch, match)
		})
	}
}

func TestHashKey(t *testing.T) {
	t.Parallel()

	// Generate a user key
	userKey, err := NewUserKey()
	assert.NoError(t, err)

	secret := "test-hmac-secret-for-testing-must-be-at-least-32-chars"

	// Hash the key
	hashedKey := userKey.HashKey(secret)

	// Check that the hashed key is not empty
	assert.NotEmpty(t, hashedKey)

	// Check that the original key and the hashed key are not equal
	assert.NotEqual(t, userKey, Key(hashedKey))

	// Verify the hashed key
	valid := userKey.ValidateHash(hashedKey, secret)
	assert.True(t, valid)

	// Test failure case: modified key should not validate
	modifiedKey, err := NewUserKey()
	assert.NoError(t, err)
	valid = modifiedKey.ValidateHash(hashedKey, secret)
	assert.False(t, valid)

	// Test failure case: empty hash should fail
	valid = userKey.ValidateHash("", secret)
	assert.False(t, valid)

	// Test failure case: invalid hash format should fail
	valid = userKey.ValidateHash("not-an-hmac-hash", secret)
	assert.False(t, valid)
}

func TestExtractUnobfuscatedPart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		key      Key
		expected string
	}{
		{
			name:     "fully obfuscated key with prefix and suffix",
			key:      Key("xau_****************1234"),
			expected: "xau_1234",
		},
		{
			name:     "key with no obfuscation",
			key:      Key("xau_abcdef123456"),
			expected: "xau_abcdef123456",
		},
		{
			name:     "key with mixed characters",
			key:      Key("xao_ab**cd**ef"),
			expected: "xao_abcdef",
		},
		{
			name:     "key with only asterisks",
			key:      Key("*****"),
			expected: "",
		},
		{
			name:     "empty key",
			key:      Key(""),
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := tc.key.extractUnobfuscatedPart()
			assert.Equal(t, tc.expected, result)
		})
	}
}
