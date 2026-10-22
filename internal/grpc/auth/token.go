// Package grpcauth authenticates service-to-service gRPC calls with short-lived
// Ed25519-signed JWTs. Callers hold a private key and attach a bearer token to
// every RPC through TokenSource; servers hold the matching public keys and
// verify tokens through Verifier, typically via the Interceptor.
//
// Public keys are the only material that has to be distributed to the
// verifying side, which keeps rollouts to many cells cheap: no secret has to
// be shared, and rotation is "trust the new key, switch the signer, drop the
// old key".
package grpcauth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// HeaderKey is the metadata key carrying the bearer token.
	HeaderKey = "authorization"
	// DefaultTTL is the lifetime of a minted token.
	DefaultTTL = 5 * time.Minute

	// refreshBefore is how long before expiry a cached token is re-minted, so
	// a token never reaches the server within its last minute of validity.
	refreshBefore = 1 * time.Minute
	// notBeforeSlack tolerates a verifier whose clock runs slightly behind.
	notBeforeSlack = 30 * time.Second
	kidHeader      = "kid"
)

// Claims carried by a service token. Issuer and Subject name the calling
// service (e.g. "projects"); Audience names the called service (e.g.
// "clusters").
type Claims struct {
	jwt.RegisteredClaims
}

// KeyID derives a stable identifier for a public key, sent in the token
// header so a verifier trusting several keys can pick the right one.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8])
}

// TokenSource mints and caches tokens. It implements
// credentials.PerRPCCredentials, so one instance can be attached to any
// number of client connections with grpc.WithPerRPCCredentials.
type TokenSource struct {
	mint *minter
	src  oauth2.TokenSource
}

// NewTokenSource returns a TokenSource signing with key on behalf of issuer
// for audience. Tokens live for ttl (DefaultTTL when zero).
func NewTokenSource(key ed25519.PrivateKey, issuer, audience string, ttl time.Duration) *TokenSource {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	pub, _ := key.Public().(ed25519.PublicKey)
	mint := &minter{
		key:      key,
		kid:      KeyID(pub),
		issuer:   issuer,
		audience: audience,
		ttl:      ttl,
		now:      time.Now,
	}
	return &TokenSource{
		mint: mint,
		src:  oauth2.ReuseTokenSourceWithExpiry(nil, mint, refreshBefore),
	}
}

// GetRequestMetadata implements credentials.PerRPCCredentials.
func (t *TokenSource) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	tok, err := t.Token()
	if err != nil {
		// codes.Internal is one of the codes gRFC A54 allows a
		// PerRPCCredentials implementation to surface.
		return nil, status.Errorf(codes.Internal, "mint grpc auth token: %v", err)
	}
	return map[string]string{HeaderKey: "Bearer " + tok}, nil
}

// RequireTransportSecurity implements credentials.PerRPCCredentials. It
// reports false so the same source works on plaintext and TLS connections
// during a transport migration; whether a token may travel in plaintext is
// decided by the server's TLS mode instead.
func (t *TokenSource) RequireTransportSecurity() bool {
	return false
}

// Token returns a valid token, minting a new one when the cached one is
// about to expire.
func (t *TokenSource) Token() (string, error) {
	tok, err := t.src.Token()
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// minter is the oauth2.TokenSource that mints a fresh token on every call;
// oauth2.ReuseTokenSourceWithExpiry adds the caching on top.
type minter struct {
	key      ed25519.PrivateKey
	kid      string
	issuer   string
	audience string
	ttl      time.Duration
	now      func() time.Time
}

func (m *minter) Token() (*oauth2.Token, error) {
	now := m.now()
	exp := now.Add(m.ttl)
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return nil, fmt.Errorf("generate token id: %w", err)
	}
	claims := Claims{jwt.RegisteredClaims{
		Issuer:    m.issuer,
		Subject:   m.issuer,
		Audience:  jwt.ClaimStrings{m.audience},
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now.Add(-notBeforeSlack)),
		ExpiresAt: jwt.NewNumericDate(exp),
		ID:        hex.EncodeToString(jti),
	}}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header[kidHeader] = m.kid

	signed, err := tok.SignedString(m.key)
	if err != nil {
		return nil, fmt.Errorf("sign token: %w", err)
	}
	return &oauth2.Token{AccessToken: signed, Expiry: exp}, nil
}

// Verifier validates tokens against a set of trusted public keys.
type Verifier struct {
	keys     map[string]ed25519.PublicKey
	audience string
	leeway   time.Duration
	now      func() time.Time
}

// NewVerifier returns a Verifier trusting keys, requiring audience and
// tolerating leeway of clock skew on time-based claims.
func NewVerifier(keys []ed25519.PublicKey, audience string, leeway time.Duration) *Verifier {
	v := &Verifier{
		keys:     make(map[string]ed25519.PublicKey, len(keys)),
		audience: audience,
		leeway:   leeway,
		now:      time.Now,
	}
	for _, k := range keys {
		v.keys[KeyID(k)] = k
	}
	return v
}

// KeyIDs lists the identifiers of the trusted keys, for startup logs.
func (v *Verifier) KeyIDs() []string {
	ids := make([]string, 0, len(v.keys))
	for id := range v.keys {
		ids = append(ids, id)
	}
	return ids
}

// Verify parses and validates raw, returning its claims.
func (v *Verifier) Verify(raw string) (*Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(raw, &claims, v.keyFunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(v.leeway),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	return &claims, nil
}

func (v *Verifier) keyFunc(tok *jwt.Token) (any, error) {
	kid, _ := tok.Header[kidHeader].(string)
	key, ok := v.keys[kid]
	if !ok {
		return nil, fmt.Errorf("unknown key id %q", kid)
	}
	return key, nil
}
