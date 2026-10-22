package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net"
	"testing"
	"time"

	clustersv1 "xata/gen/proto/clusters/v1"
	"xata/internal/envcfg"
	grpcauth "xata/internal/grpc/auth"
	"xata/internal/o11y"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type stubClusters struct {
	clustersv1.UnimplementedClustersServiceServer
}

func (s *stubClusters) DescribePostgresCluster(ctx context.Context, _ *clustersv1.DescribePostgresClusterRequest) (*clustersv1.DescribePostgresClusterResponse, error) {
	caller, _ := grpcauth.CallerFromContext(ctx)
	return &clustersv1.DescribePostgresClusterResponse{Id: caller}, nil
}

func startServer(t *testing.T, cfg Config) string {
	t.Helper()
	cfg.MaxConcurrentStreams = 10
	cfg.ConnectionTimeout = 10 * time.Second
	o := o11y.NewTestService(t)
	server := SetupGRPC(&o, cfg)
	clustersv1.RegisterClustersServiceServer(server, &stubClusters{})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = server.Serve(lis) }()
	t.Cleanup(server.Stop)
	return lis.Addr().String()
}

func TestSetupGRPCAuth(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	tests := map[string]struct {
		mode       string
		signer     ed25519.PrivateKey
		wantCode   codes.Code
		wantCaller string
	}{
		"off without token":          {mode: "off"},
		"log without token":          {mode: "log"},
		"log with token":             {mode: "log", signer: priv, wantCaller: "projects"},
		"enforce without token":      {mode: "enforce", wantCode: codes.Unauthenticated},
		"enforce with untrusted key": {mode: "enforce", signer: otherPriv, wantCode: codes.Unauthenticated},
		"enforce with token":         {mode: "enforce", signer: priv, wantCaller: "projects"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			addr := startServer(t, Config{
				GRPCAuthMode:        tt.mode,
				GRPCAuthTrustedKeys: envcfg.PublicEDDSAKeys{pub},
				GRPCAuthAudience:    "clusters",
				GRPCAuthLeeway:      time.Minute,
			})

			opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
			if tt.signer != nil {
				opts = append(opts, grpc.WithPerRPCCredentials(grpcauth.NewTokenSource(tt.signer, "projects", "clusters", 0)))
			}
			conn, err := grpc.NewClient(addr, opts...)
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Health stays reachable without credentials in every mode, so
			// kubelet probes keep working.
			hres, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
			require.NoError(t, err)
			require.Equal(t, healthpb.HealthCheckResponse_SERVING, hres.GetStatus())

			res, err := clustersv1.NewClustersServiceClient(conn).DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{})
			require.Equal(t, tt.wantCode, status.Code(err), "%v", err)
			if tt.wantCode == codes.OK {
				require.Equal(t, tt.wantCaller, res.GetId())
			}
		})
	}
}

func TestConfigAfterRead(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	tests := map[string]struct {
		cfg     Config
		wantErr bool
	}{
		"off needs nothing":        {cfg: Config{GRPCAuthMode: "off"}},
		"unknown mode":             {cfg: Config{GRPCAuthMode: "sometimes"}, wantErr: true},
		"log without keys":         {cfg: Config{GRPCAuthMode: "log", GRPCAuthAudience: "clusters"}, wantErr: true},
		"enforce without audience": {cfg: Config{GRPCAuthMode: "enforce", GRPCAuthTrustedKeys: envcfg.PublicEDDSAKeys{pub}}, wantErr: true},
		"enforce complete":         {cfg: Config{GRPCAuthMode: "enforce", GRPCAuthTrustedKeys: envcfg.PublicEDDSAKeys{pub}, GRPCAuthAudience: "clusters"}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := tt.cfg.AfterRead()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestConfigReadFromEnv(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	var keys envcfg.PublicEDDSAKeys
	require.NoError(t, keys.SetValue(publicKeyPEM(t, pub)))

	t.Setenv("XATA_GRPC_AUTH_MODE", "enforce")
	t.Setenv("XATA_GRPC_AUTH_AUDIENCE", "clusters")
	t.Setenv("XATA_GRPC_AUTH_TRUSTED_KEYS", publicKeyPEM(t, pub))

	var cfg Config
	require.NoError(t, envcfg.Read(&cfg))
	require.Equal(t, "enforce", cfg.GRPCAuthMode)
	require.Len(t, cfg.GRPCAuthTrustedKeys, 1)
	require.True(t, pub.Equal(cfg.GRPCAuthTrustedKeys[0]))

	t.Setenv("XATA_GRPC_AUTH_TRUSTED_KEYS", "")
	var incomplete Config
	require.Error(t, envcfg.Read(&incomplete), "AfterRead must reject enforce without keys")
}

func publicKeyPEM(t *testing.T, pub ed25519.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}
