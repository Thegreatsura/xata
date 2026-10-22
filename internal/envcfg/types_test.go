package envcfg

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

func TestTolerationListFieldSetValue(t *testing.T) {
	tests := map[string]struct {
		raw     string
		want    []v1.Toleration
		wantErr bool
	}{
		"one toleration": {
			raw: "foo=bar:NoSchedule",
			want: []v1.Toleration{
				{
					Key:      "foo",
					Operator: v1.TolerationOpEqual,
					Value:    "bar",
					Effect:   v1.TaintEffect("NoSchedule"),
				},
			},
		},
		"missing effect": {
			raw:     "foo=bar",
			wantErr: true,
		},
		"missing value and effect": {
			raw:     "foo",
			wantErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tl := TolerationListField{}
			err := tl.SetValue(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, tl.Value)
		})
	}
}

func TestPublicEDDSAKeysSetValue(t *testing.T) {
	pub1, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	pub2, priv2, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	pem1 := publicKeyPEM(t, pub1)
	pem2 := publicKeyPEM(t, pub2)
	cert2 := certificatePEM(t, pub2, priv2)

	tests := map[string]struct {
		raw     string
		want    []ed25519.PublicKey
		wantErr bool
	}{
		"empty leaves keys unset":   {raw: ""},
		"single key":                {raw: pem1, want: []ed25519.PublicKey{pub1}},
		"bundle":                    {raw: pem1 + pem2, want: []ed25519.PublicKey{pub1, pub2}},
		"certificate":               {raw: cert2, want: []ed25519.PublicKey{pub2}},
		"key and certificate":       {raw: pem1 + cert2, want: []ed25519.PublicKey{pub1, pub2}},
		"escaped newlines":          {raw: strings.ReplaceAll(pem1, "\n", "\\n"), want: []ed25519.PublicKey{pub1}},
		"no pem blocks":             {raw: "garbage", wantErr: true},
		"non ed25519 key in bundle": {raw: pem1 + rsaPublicKeyPEM(t), wantErr: true},
		"non ed25519 certificate":   {raw: rsaCertificatePEM(t), wantErr: true},
		"corrupt block":             {raw: "-----BEGIN PUBLIC KEY-----\nAAAA\n-----END PUBLIC KEY-----\n", wantErr: true},
		"corrupt certificate":       {raw: "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n", wantErr: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var keys PublicEDDSAKeys
			err := keys.SetValue(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, keys, len(tt.want))
			for i, want := range tt.want {
				require.True(t, want.Equal(keys[i]))
			}
		})
	}
}

func publicKeyPEM(t *testing.T, pub ed25519.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func rsaPublicKeyPEM(t *testing.T) string {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func certificatePEM(t *testing.T, pub, priv any) string {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "projects"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func rsaCertificatePEM(t *testing.T) string {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return certificatePEM(t, &k.PublicKey, k)
}
