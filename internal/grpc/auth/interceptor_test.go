package grpcauth

import (
	"context"
	"crypto/ed25519"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func TestParseModes(t *testing.T) {
	m, err := ParseMode("")
	require.NoError(t, err)
	require.Equal(t, ModeOff, m)

	m, err = ParseMode("Enforce")
	require.NoError(t, err)
	require.Equal(t, ModeEnforce, m)
	_, err = ParseMode("maybe")
	require.Error(t, err)

	tm, err := ParseTLSMode("REQUIRE")
	require.NoError(t, err)
	require.Equal(t, TLSModeRequire, tm)
	_, err = ParseTLSMode("")
	require.Error(t, err)
}

func TestNewRequiresVerifier(t *testing.T) {
	_, err := New(nil, Config{Mode: ModeEnforce})
	require.Error(t, err)
	_, err = New(nil, Config{Mode: ModeOff})
	require.NoError(t, err)
}

type tokenState int

const (
	noToken tokenState = iota
	badToken
	goodToken
)

func TestInterceptorPolicy(t *testing.T) {
	pub, priv := genKey(t)
	_, otherPriv := genKey(t)
	verifier := NewVerifier([]ed25519.PublicKey{pub}, "clusters", 0)
	good := NewTokenSource(priv, "projects", "clusters", 0)
	bad := NewTokenSource(otherPriv, "projects", "clusters", 0)

	const rpc = "/clusters.v1.ClustersService/DescribePostgresCluster"
	const health = "/grpc.health.v1.Health/Check"
	const reflection = "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo"

	tests := map[string]struct {
		mode       Mode
		tlsMode    TLSMode
		method     string
		token      tokenState
		tls        bool
		wantCode   codes.Code
		wantCaller string
	}{
		"off ignores missing token":          {mode: ModeOff, method: rpc, token: noToken},
		"off ignores bad token":              {mode: ModeOff, method: rpc, token: badToken},
		"log allows missing token":           {mode: ModeLog, method: rpc, token: noToken},
		"log allows bad token":               {mode: ModeLog, method: rpc, token: badToken},
		"log records caller":                 {mode: ModeLog, method: rpc, token: goodToken, wantCaller: "projects"},
		"enforce rejects missing token":      {mode: ModeEnforce, method: rpc, token: noToken, wantCode: codes.Unauthenticated},
		"enforce rejects bad token":          {mode: ModeEnforce, method: rpc, token: badToken, wantCode: codes.Unauthenticated},
		"enforce accepts good token":         {mode: ModeEnforce, method: rpc, token: goodToken, wantCaller: "projects"},
		"enforce exempts health":             {mode: ModeEnforce, method: health, token: noToken},
		"enforce exempts reflection":         {mode: ModeEnforce, method: reflection, token: noToken},
		"tls optional allows plaintext":      {mode: ModeEnforce, tlsMode: TLSModeOptional, method: rpc, token: goodToken, wantCaller: "projects"},
		"tls require rejects plaintext":      {mode: ModeEnforce, tlsMode: TLSModeRequire, method: rpc, token: goodToken, wantCode: codes.Unauthenticated},
		"tls require with auth off":          {mode: ModeOff, tlsMode: TLSModeRequire, method: rpc, token: noToken, wantCode: codes.Unauthenticated},
		"tls require accepts tls":            {mode: ModeEnforce, tlsMode: TLSModeRequire, method: rpc, token: goodToken, tls: true, wantCaller: "projects"},
		"tls require exempts health":         {mode: ModeEnforce, tlsMode: TLSModeRequire, method: health, token: noToken},
		"tls require still needs good token": {mode: ModeEnforce, tlsMode: TLSModeRequire, method: rpc, token: badToken, tls: true, wantCode: codes.Unauthenticated},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			i, err := New(nil, Config{Mode: tt.mode, TLSMode: tt.tlsMode, Verifier: verifier})
			require.NoError(t, err)

			ctx := context.Background()
			switch tt.token {
			case goodToken:
				ctx = withToken(t, ctx, good)
			case badToken:
				ctx = withToken(t, ctx, bad)
			case noToken:
			}
			p := &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 1234}}
			if tt.tls {
				p.AuthInfo = credentials.TLSInfo{}
			}
			ctx = peer.NewContext(ctx, p)
			// The interceptor reads the method from the context, as set by
			// the grpc server on real calls.
			ctx = grpc.NewContextWithServerTransportStream(ctx, &fakeTransportStream{method: tt.method})

			// Unary
			var gotCaller string
			called := false
			unaryHandler := func(ctx context.Context, _ any) (any, error) {
				called = true
				gotCaller, _ = CallerFromContext(ctx)
				return "resp", nil
			}
			_, err = i.Unary()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: tt.method}, unaryHandler)
			require.Equal(t, tt.wantCode, status.Code(err), "unary: %v", err)
			require.Equal(t, tt.wantCode == codes.OK, called)
			require.Equal(t, tt.wantCaller, gotCaller)

			// Stream
			gotCaller, called = "", false
			streamHandler := func(_ any, stream grpc.ServerStream) error {
				called = true
				gotCaller, _ = CallerFromContext(stream.Context())
				return nil
			}
			err = i.Stream()(nil, &fakeStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: tt.method}, streamHandler)
			require.Equal(t, tt.wantCode, status.Code(err), "stream: %v", err)
			require.Equal(t, tt.wantCode == codes.OK, called)
			require.Equal(t, tt.wantCaller, gotCaller)
		})
	}
}

func withToken(t *testing.T, ctx context.Context, src *TokenSource) context.Context {
	t.Helper()
	md, err := src.GetRequestMetadata(ctx)
	require.NoError(t, err)
	return metadata.NewIncomingContext(ctx, metadata.New(md))
}

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeStream) Context() context.Context { return f.ctx }

type fakeTransportStream struct{ method string }

func (f *fakeTransportStream) Method() string               { return f.method }
func (f *fakeTransportStream) SetHeader(metadata.MD) error  { return nil }
func (f *fakeTransportStream) SendHeader(metadata.MD) error { return nil }
func (f *fakeTransportStream) SetTrailer(metadata.MD) error { return nil }
