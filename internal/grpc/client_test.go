package grpc_test

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"xata/internal/api"
	internalgrpc "xata/internal/grpc"
	"xata/internal/o11y"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
)

func TestNewClientReconnectsAfterPacketLoss(t *testing.T) {
	o := o11y.NewTestService(t)
	startServer := func() string {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		server := api.SetupGRPC(&o, api.Config{ConnectionTimeout: time.Minute})
		serveErr := make(chan error, 1)
		go func() { serveErr <- server.Serve(listener) }()
		t.Cleanup(func() {
			server.Stop()
			require.NoError(t, <-serveErr)
		})
		return listener.Addr().String()
	}
	oldAddress := startServer()
	newAddress := startServer()
	var drop atomic.Bool
	var refreshes atomic.Int32
	r := manual.NewBuilderWithScheme("proxy")
	r.InitialState(resolver.State{Addresses: []resolver.Address{{Addr: oldAddress}}})
	r.ResolveNowCallback = func(resolver.ResolveNowOptions) {
		if drop.Load() {
			refreshes.Add(1)
			r.UpdateState(resolver.State{Addresses: []resolver.Address{{Addr: newAddress}}})
		}
	}
	conn, err := internalgrpc.NewClient(&o, "proxy:///cell",
		grpc.WithResolvers(r),
		grpc.WithContextDialer(func(ctx context.Context, address string) (net.Conn, error) {
			conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
			if err != nil {
				return nil, err
			}
			if address == oldAddress {
				return &packetLossConn{Conn: conn, drop: &drop}, nil
			}
			return conn, nil
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	client := healthpb.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err = client.Check(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)

	// Recovery must detect a silent peer without a socket error.
	drop.Store(true)
	ctx, cancel = context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	response, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, response.Status)
	require.Positive(t, refreshes.Load())
}

type packetLossConn struct {
	net.Conn
	drop *atomic.Bool
}

func (c *packetLossConn) Write(p []byte) (int, error) {
	if c.drop.Load() {
		return len(p), nil
	}
	return c.Conn.Write(p)
}
