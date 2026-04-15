package envcfg

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/dustin/go-humanize"
	"github.com/golang-jwt/jwt/v5"
)

type ByteSize uint64

func (b *ByteSize) SetValue(in string) error {
	tmp, err := humanize.ParseBytes(in)
	*b = ByteSize(tmp)
	return err
}

type PrivateKey struct {
	key *rsa.PrivateKey
}

func GeneratePrivateKey() (k PrivateKey, err error) {
	k.key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return k, fmt.Errorf("generate private key: %w", err)
	}
	return k, err
}

func (p *PrivateKey) SetValue(in string) (err error) {
	if in != "" {
		p.key, err = jwt.ParseRSAPrivateKeyFromPEM(readMultilinePEM(in))
		if err != nil {
			err = fmt.Errorf("parse private key: %w", err)
		}
	}
	return err
}

func (p *PrivateKey) IsZero() bool { return p.key == nil }

func (p *PrivateKey) Set(k *rsa.PrivateKey) {
	p.key = k
}

func (p *PrivateKey) RSAKey() *rsa.PrivateKey {
	return p.key
}

func (p *PrivateKey) Public() *rsa.PublicKey {
	if p == nil || p.key == nil {
		return nil
	}
	return &p.key.PublicKey
}

type PublicKey struct {
	key *rsa.PublicKey
}

func (p *PublicKey) IsZero() bool { return p.key == nil }

func (p *PublicKey) Set(k *rsa.PublicKey) {
	p.key = k
}

func (p *PublicKey) SetValue(in string) (err error) {
	if in != "" {
		p.key, err = jwt.ParseRSAPublicKeyFromPEM(readMultilinePEM(in))
		if err != nil {
			err = fmt.Errorf("parse public key: %w", err)
		}
	}
	return err
}

func (p *PublicKey) RSAKey() *rsa.PublicKey {
	return p.key
}

func readMultilinePEM(key string) []byte {
	return []byte(strings.ReplaceAll(key, "\\n", "\n"))
}

type PrivateEDDSAKey ed25519.PrivateKey

func GeneratePrivateEDDSAKey() (k PrivateEDDSAKey, err error) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return k, fmt.Errorf("generate private key: %w", err)
	}
	return PrivateEDDSAKey(key), err
}

func (p *PrivateEDDSAKey) SetValue(in string) error {
	if in != "" {
		k, err := jwt.ParseEdPrivateKeyFromPEM(readMultilinePEM(in))
		if err != nil {
			return fmt.Errorf("parse private eddsa key: %w", err)
		}
		pkey, ok := k.(ed25519.PrivateKey)
		if !ok {
			return fmt.Errorf("unexpected key type")
		}
		*p = PrivateEDDSAKey(pkey)
	}
	return nil
}

func (p *PrivateEDDSAKey) IsZero() bool {
	return p != nil && len(*p) == 0
}

func (p *PrivateEDDSAKey) Set(k ed25519.PrivateKey) {
	*p = PrivateEDDSAKey(k)
}

func (p *PrivateEDDSAKey) Key() crypto.PrivateKey {
	return ed25519.PrivateKey(*p)
}

func (p *PrivateEDDSAKey) Public() ed25519.PublicKey {
	if p == nil {
		return nil
	}
	return ed25519.PrivateKey(*p).Public().(ed25519.PublicKey) //nolint:forcetypeassert
}

type PublicEDDSAKey ed25519.PublicKey

func (p *PublicEDDSAKey) IsZero() bool {
	return p != nil && len(*p) == 0
}

func (p *PublicEDDSAKey) Set(k ed25519.PublicKey) {
	*p = PublicEDDSAKey(k)
}

func (p *PublicEDDSAKey) SetValue(in string) error {
	if in != "" {
		k, err := jwt.ParseEdPublicKeyFromPEM(readMultilinePEM(in))
		if err != nil {
			return fmt.Errorf("parse public eddsa key: %w", err)
		}
		pkey, ok := k.(ed25519.PublicKey)
		if !ok {
			return fmt.Errorf("unexpected key type")
		}
		*p = PublicEDDSAKey(pkey)
	}
	return nil
}

func (p *PublicEDDSAKey) Key() crypto.PublicKey {
	return ed25519.PublicKey(*p)
}

type CACertPool x509.CertPool

func (p *CACertPool) SetValue(in string) {
	caCertPEM, _ := readPEMFile(in)
	if caCertPEM != nil {
		cp := x509.NewCertPool()
		cp.AppendCertsFromPEM(caCertPEM)
		*p = CACertPool(*cp)
	}
}

func (p *CACertPool) IsZero() bool {
	return p == nil
}

func (p *CACertPool) Certs() *x509.CertPool {
	if p == nil {
		return nil
	}

	pool := x509.CertPool(*p)
	return &pool
}

type ClientPEMCertificate []byte

func (c *ClientPEMCertificate) SetValue(in string) {
	*c, _ = readPEMFile(in)
}

func readPEMFile(s string) ([]byte, error) {
	var blocks []*pem.Block

	r, err := newPEMReader(s)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	content, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	for len(content) > 0 {
		var block *pem.Block

		block, content = pem.Decode(content)
		if block == nil {
			if len(blocks) == 0 {
				return nil, fmt.Errorf("no pem file")
			}
			break
		}

		blocks = append(blocks, block)
	}

	if len(blocks) == 0 {
		return nil, fmt.Errorf("no PEM blocks")
	}

	// re-encode available, decrypted blocks
	buffer := bytes.NewBuffer(nil)
	for _, block := range blocks {
		err := pem.Encode(buffer, block)
		if err != nil {
			return nil, err
		}
	}
	return buffer.Bytes(), nil
}

// pemReader allows to read a certificate in PEM format either through the disk or from a string.
type pemReader struct {
	reader   io.ReadCloser
	debugStr string
}

// newPEMReader returns a new pemReader.
func newPEMReader(certificate string) (*pemReader, error) {
	if isPEMString(certificate) {
		return &pemReader{reader: io.NopCloser(strings.NewReader(certificate)), debugStr: "inline"}, nil
	}

	r, err := os.Open(certificate)
	if err != nil {
		return nil, err
	}
	return &pemReader{reader: r, debugStr: certificate}, nil
}

// Close closes the target io.ReadCloser.
func (p *pemReader) Close() error {
	return p.reader.Close()
}

// Read read bytes from the io.ReadCloser.
func (p *pemReader) Read(b []byte) (n int, err error) {
	return p.reader.Read(b)
}

func (p *pemReader) String() string {
	return p.debugStr
}

// IsPEMString returns true if the provided string match a PEM formatted certificate. try to pem decode to validate.
func isPEMString(s string) bool {
	// Trim the certificates to make sure we tolerate any yaml weirdness, we assume that the string starts
	// with "-" and let further validation verifies the PEM format. When migrating from pkcs12 to PEM a "Bag Attributes" header is added.
	trimmedStr := strings.TrimSpace(s)
	return strings.HasPrefix(trimmedStr, "-") || strings.HasPrefix(trimmedStr, "Bag Attributes")
}

type QuantityField struct {
	Value resource.Quantity
}

func (q *QuantityField) SetValue(in string) error {
	parsed, err := resource.ParseQuantity(in)
	if err != nil {
		return fmt.Errorf("invalid quantity %q: %w", in, err)
	}
	q.Value = parsed
	return nil
}

func (q QuantityField) String() string {
	return q.Value.String()
}

type TolerationListField struct {
	Value []v1.Toleration
}

func (tl *TolerationListField) SetValue(raw []string) error {
	list := make([]v1.Toleration, 0, len(raw))

	for _, entry := range raw {
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
