package envcfg

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"

	"github.com/golang-jwt/jwt/v5"
)

// PublicEDDSAKeys holds one or more Ed25519 public keys read from a PEM
// bundle of bare public keys or X.509 certificates (e.g. issued by
// cert-manager), so a server can trust several signers at once (e.g. during
// key rotation, or one key per calling service).
type PublicEDDSAKeys []ed25519.PublicKey

func (p *PublicEDDSAKeys) SetValue(in string) error {
	if in == "" {
		return nil
	}
	rest := []byte(strings.ReplaceAll(in, "\\n", "\n"))
	var keys PublicEDDSAKeys
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		pkey, err := publicEDDSAKey(block)
		if err != nil {
			return fmt.Errorf("parse public eddsa key %d: %w", len(keys)+1, err)
		}
		keys = append(keys, pkey)
	}
	if len(keys) == 0 {
		return fmt.Errorf("no pem blocks found in public eddsa keys")
	}
	*p = keys
	return nil
}

// publicEDDSAKey extracts the Ed25519 public key from a PEM block holding
// either the bare key or an X.509 certificate.
func publicEDDSAKey(block *pem.Block) (ed25519.PublicKey, error) {
	if block.Type == "CERTIFICATE" {
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse certificate: %w", err)
		}
		pkey, ok := cert.PublicKey.(ed25519.PublicKey)
		if !ok {
			return nil, fmt.Errorf("certificate holds a %T, want an ed25519 key", cert.PublicKey)
		}
		return pkey, nil
	}
	k, err := jwt.ParseEdPublicKeyFromPEM(pem.EncodeToMemory(block))
	if err != nil {
		return nil, err
	}
	pkey, ok := k.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("unexpected key type")
	}
	return pkey, nil
}

func (p *PublicEDDSAKeys) IsZero() bool {
	return p != nil && len(*p) == 0
}

type TolerationListField struct {
	Value []v1.Toleration
}

func (tl *TolerationListField) SetValue(raw string) error {
	if raw == "" {
		return fmt.Errorf("tolerations are required but not set")
	}

	entries := strings.Split(raw, ",")
	list := make([]v1.Toleration, 0, len(entries))

	for _, entry := range entries {
		// Format: key=value:effect
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid toleration format: %s", entry)
		}
		keyValue := strings.SplitN(parts[0], "=", 2)
		if len(keyValue) != 2 {
			return fmt.Errorf("invalid key=value in toleration: %s", parts[0])
		}
		t := v1.Toleration{
			Key:      keyValue[0],
			Value:    keyValue[1],
			Operator: v1.TolerationOpEqual,
			Effect:   v1.TaintEffect(parts[1]),
		}
		list = append(list, t)
	}
	tl.Value = list
	return nil
}
