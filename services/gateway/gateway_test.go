package gateway

import (
	"testing"

	"xata/internal/service"

	"github.com/stretchr/testify/require"
)

// TestGatewayServiceIsRunnerOnly pins the invariant that GatewayService
// registers nothing but RunnerService in internal/cmd/run.go. The ctx passed
// to Run is an errgroup context shared by all registered sub-services; it is
// only cancelled exclusively by a shutdown signal while the gateway is the
// sole member. Adding HTTPService or GRPCService would make a sibling failure
// cancel it too, and the server would misread that as a graceful shutdown and
// drain for up to DrainingTime instead of failing fast (see
// ServerConfig.ShutdownSignal). If you need one of these interfaces, plumb
// the signal-only context from run.go into newServer first.
func TestGatewayServiceIsRunnerOnly(t *testing.T) {
	var svc any = &GatewayService{}

	_, ok := svc.(service.RunnerService)
	require.True(t, ok, "GatewayService must implement RunnerService")

	_, ok = svc.(service.HTTPService)
	require.False(t, ok, "GatewayService must not implement HTTPService: ServerConfig.ShutdownSignal would become sibling-cancellable")

	_, ok = svc.(service.GRPCService)
	require.False(t, ok, "GatewayService must not implement GRPCService: ServerConfig.ShutdownSignal would become sibling-cancellable")
}
