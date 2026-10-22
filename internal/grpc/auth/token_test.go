package grpcauth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func genKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

func TestTokenRoundTrip(t *testing.T) {
	pub, priv := genKey(t)
	otherPub, _ := genKey(t)
	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		trusted   []ed25519.PublicKey
		audience  string
		verifyAt  time.Time
		leeway    time.Duration
		wantErr   bool
		wantIssue string
	}{
		"valid": {
			trusted: []ed25519.PublicKey{pub}, audience: "clusters", verifyAt: base, wantIssue: "projects",
		},
		"several trusted keys": {
			trusted: []ed25519.PublicKey{otherPub, pub}, audience: "clusters", verifyAt: base, wantIssue: "projects",
		},
		"untrusted key": {
			trusted: []ed25519.PublicKey{otherPub}, audience: "clusters", verifyAt: base, wantErr: true,
		},
		"wrong audience": {
			trusted: []ed25519.PublicKey{pub}, audience: "other", verifyAt: base, wantErr: true,
		},
		"expired within leeway": {
			trusted: []ed25519.PublicKey{pub}, audience: "clusters", verifyAt: base.Add(DefaultTTL + 30*time.Second), leeway: time.Minute, wantIssue: "projects",
		},
		"expired beyond leeway": {
			trusted: []ed25519.PublicKey{pub}, audience: "clusters", verifyAt: base.Add(DefaultTTL + 2*time.Minute), leeway: time.Minute, wantErr: true,
		},
		"issued in the future beyond leeway": {
			trusted: []ed25519.PublicKey{pub}, audience: "clusters", verifyAt: base.Add(-2 * time.Minute), leeway: time.Minute, wantErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			src := NewTokenSource(priv, "projects", "clusters", 0)
			src.mint.now = func() time.Time { return base }
			tok, err := src.Token()
			require.NoError(t, err)

			v := NewVerifier(tt.trusted, tt.audience, tt.leeway)
			v.now = func() time.Time { return tt.verifyAt }
			got, err := v.Verify(tok)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantIssue, got.Issuer)
			require.Equal(t, tt.wantIssue, got.Subject)
		})
	}
}

func TestVerifierRejectsGarbage(t *testing.T) {
	pub, _ := genKey(t)
	v := NewVerifier([]ed25519.PublicKey{pub}, "clusters", 0)
	_, err := v.Verify("not.a.jwt")
	require.Error(t, err)
}

func TestTokenSourceCaches(t *testing.T) {
	_, priv := genKey(t)

	// Well before expiry the cached token is reused.
	src := NewTokenSource(priv, "projects", "clusters", 0)
	first, err := src.Token()
	require.NoError(t, err)
	again, err := src.Token()
	require.NoError(t, err)
	require.Equal(t, first, again)

	// A token within refreshBefore of expiry is not reused: with a ttl
	// inside that window every call mints a fresh token.
	src = NewTokenSource(priv, "projects", "clusters", refreshBefore/2)
	first, err = src.Token()
	require.NoError(t, err)
	fresh, err := src.Token()
	require.NoError(t, err)
	require.NotEqual(t, first, fresh)
}

func TestTokenSourceConcurrent(t *testing.T) {
	pub, priv := genKey(t)
	src := NewTokenSource(priv, "projects", "clusters", 0)
	v := NewVerifier([]ed25519.PublicKey{pub}, "clusters", 0)

	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for range 50 {
		wg.Go(func() {
			md, err := src.GetRequestMetadata(context.Background())
			if err != nil {
				errs <- err
				return
			}
			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(md))
			raw, err := auth.AuthFromMD(ctx, "bearer")
			if err != nil {
				errs <- err
				return
			}
			if _, err := v.Verify(raw); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.False(t, src.RequireTransportSecurity())
}
