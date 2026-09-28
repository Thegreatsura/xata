package secretcipher

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const testSecret = "access_token_abc123"

// testKey returns a valid base64-encoded 32-byte key.
func testKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, KeyLen)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(key)
}

func newTestCipher(t *testing.T) *Cipher {
	t.Helper()
	c, err := New(testKey(t))
	require.NoError(t, err)
	return c
}

func TestNew(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString(make([]byte, 16))

	tests := map[string]struct {
		key     string
		wantErr bool
	}{
		"valid 32-byte key": {key: testKey(t)},
		"empty key":         {key: "", wantErr: true},
		"not base64":        {key: "not-base64-!!!", wantErr: true},
		"wrong key length":  {key: shortKey, wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := New(tc.key)
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
		})
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c := newTestCipher(t)

	tests := map[string]string{
		"typical secret": testSecret,
		"long secret":    strings.Repeat("a", 4096),
		"unicode":        "token-ü-🔑",
	}

	for name, secret := range tests {
		t.Run(name, func(t *testing.T) {
			encrypted, err := c.Encrypt(secret, []byte("row-1"))
			require.NoError(t, err)
			require.NotContains(t, encrypted, secret, "plaintext must not appear in ciphertext")
			require.True(t, strings.HasPrefix(encrypted, versionV1+":"), "must carry the version prefix")

			got, err := c.Decrypt(encrypted, []byte("row-1"))
			require.NoError(t, err)
			require.Equal(t, secret, got)
		})
	}
}

func TestEncryptRejectsEmptyPlaintext(t *testing.T) {
	c := newTestCipher(t)
	_, err := c.Encrypt("", nil)
	require.Error(t, err)
}

func TestEncryptUsesFreshNonce(t *testing.T) {
	c := newTestCipher(t)

	first, err := c.Encrypt(testSecret, nil)
	require.NoError(t, err)
	second, err := c.Encrypt(testSecret, nil)
	require.NoError(t, err)

	require.NotEqual(t, first, second, "same secret must not encrypt to the same value twice")

	for _, encrypted := range []string{first, second} {
		got, err := c.Decrypt(encrypted, nil)
		require.NoError(t, err)
		require.Equal(t, testSecret, got)
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	encrypted, err := newTestCipher(t).Encrypt(testSecret, nil)
	require.NoError(t, err)

	_, err = newTestCipher(t).Decrypt(encrypted, nil)
	require.Error(t, err)
}

func TestDecryptRejectsWrongAssociatedData(t *testing.T) {
	c := newTestCipher(t)
	encrypted, err := c.Encrypt(testSecret, []byte("user-a/conn-1"))
	require.NoError(t, err)

	_, err = c.Decrypt(encrypted, []byte("user-b/conn-1"))
	require.Error(t, err)
	_, err = c.Decrypt(encrypted, nil)
	require.Error(t, err)
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	c := newTestCipher(t)
	encrypted, err := c.Encrypt(testSecret, nil)
	require.NoError(t, err)

	_, payload, found := strings.Cut(encrypted, ":")
	require.True(t, found)
	raw, err := base64.StdEncoding.DecodeString(payload)
	require.NoError(t, err)

	tests := map[string]func([]byte){
		"flip a ciphertext bit": func(b []byte) { b[len(b)-1] ^= 0x01 },
		"flip a nonce bit":      func(b []byte) { b[0] ^= 0x01 },
	}

	for name, tamper := range tests {
		t.Run(name, func(t *testing.T) {
			corrupted := make([]byte, len(raw))
			copy(corrupted, raw)
			tamper(corrupted)

			_, err := c.Decrypt(versionV1+":"+base64.StdEncoding.EncodeToString(corrupted), nil)
			require.Error(t, err)
		})
	}
}

func TestDecryptRejectsMalformedInput(t *testing.T) {
	c := newTestCipher(t)
	shortPayload := base64.StdEncoding.EncodeToString(make([]byte, nonceLen-1))

	tests := map[string]string{
		"empty":              "",
		"no version prefix":  base64.StdEncoding.EncodeToString([]byte("whatever")),
		"unknown version":    "v99:" + base64.StdEncoding.EncodeToString([]byte("whatever")),
		"payload not base64": versionV1 + ":not-base64-!!!",
		"shorter than nonce": versionV1 + ":" + shortPayload,
	}

	for name, stored := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := c.Decrypt(stored, nil)
			require.Error(t, err)
		})
	}
}

func TestDecryptErrorDoesNotLeakSecrets(t *testing.T) {
	encrypted, err := newTestCipher(t).Encrypt(testSecret, nil)
	require.NoError(t, err)

	_, err = newTestCipher(t).Decrypt(encrypted, nil)
	require.Error(t, err)
	require.NotContains(t, err.Error(), testSecret)
}
