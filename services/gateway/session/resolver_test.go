package session_test

import (
	"context"
	"testing"

	"xata/services/gateway/session"

	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	resolver := session.NewCNPGBranchResolver("test-namespace", 5432, false)

	tests := map[string]struct {
		serverName string
		fallback   string
		wantAddr   string
		wantBranch string
	}{
		"simple branch": {
			serverName: "branch1.example.com",
			fallback:   session.EndpointRW,
			wantAddr:   "branch-branch1-rw.test-namespace.svc.cluster.local:5432",
			wantBranch: "branch1",
		},
		"read-only endpoint": {
			serverName: "branch1-ro.example.com",
			fallback:   session.EndpointRW,
			wantAddr:   "branch-branch1-ro.test-namespace.svc.cluster.local:5432",
			wantBranch: "branch1",
		},
		"read-write endpoint explicitly requested": {
			serverName: "branch1-rw.example.com",
			fallback:   session.EndpointRW,
			wantAddr:   "branch-branch1-rw.test-namespace.svc.cluster.local:5432",
			wantBranch: "branch1",
		},
		"read endpoint": {
			serverName: "branch1-r.example.com",
			fallback:   session.EndpointRW,
			wantAddr:   "branch-branch1-r.test-namespace.svc.cluster.local:5432",
			wantBranch: "branch1",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			branch, err := resolver.Resolve(context.Background(), test.serverName, test.fallback)
			require.NoError(t, err, "unexpected error")
			require.Equal(t, test.wantAddr, branch.Address, "expected address")
			require.Equal(t, test.wantBranch, branch.ID, "expected branch")
		})
	}
}

// TestResolve_PoolerEnabled are running in a separate test function
// until the connection pooler feature flag is removed or enabled in prod.
func TestResolve_PoolerEnabled(t *testing.T) {
	resolver := session.NewCNPGBranchResolver("test-namespace", 5432, true)

	tests := map[string]struct {
		serverName string
		fallback   string
		wantAddr   string
		wantBranch string
	}{
		"pooler endpoint": {
			serverName: "branch1-pooler.example.com",
			fallback:   session.EndpointRW,
			wantAddr:   "branch-branch1-pooler.test-namespace.svc.cluster.local:5432",
			wantBranch: "branch1",
		},
		"pooler with underscore branch": {
			serverName: "my_branch-pooler.example.com",
			fallback:   session.EndpointRW,
			wantAddr:   "branch-my_branch-pooler.test-namespace.svc.cluster.local:5432",
			wantBranch: "my_branch",
		},
		"rw still works with pooler enabled": {
			serverName: "branch1-rw.example.com",
			fallback:   session.EndpointRW,
			wantAddr:   "branch-branch1-rw.test-namespace.svc.cluster.local:5432",
			wantBranch: "branch1",
		},
		"rw fallback with pooler enabled": {
			serverName: "branch1.example.com",
			fallback:   session.EndpointRW,
			wantAddr:   "branch-branch1-rw.test-namespace.svc.cluster.local:5432",
			wantBranch: "branch1",
		},
		"pooler fallback with no suffix": {
			serverName: "branch1.example.com",
			fallback:   session.EndpointPooler,
			wantAddr:   "branch-branch1-pooler.test-namespace.svc.cluster.local:5432",
			wantBranch: "branch1",
		},
		"explicit -rw wins over pooler fallback": {
			serverName: "branch1-rw.example.com",
			fallback:   session.EndpointPooler,
			wantAddr:   "branch-branch1-rw.test-namespace.svc.cluster.local:5432",
			wantBranch: "branch1",
		},
		"explicit -ro wins over pooler fallback": {
			serverName: "branch1-ro.example.com",
			fallback:   session.EndpointPooler,
			wantAddr:   "branch-branch1-ro.test-namespace.svc.cluster.local:5432",
			wantBranch: "branch1",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			branch, err := resolver.Resolve(context.Background(), test.serverName, test.fallback)
			require.NoError(t, err, "unexpected error")
			require.Equal(t, test.wantAddr, branch.Address, "expected address")
			require.Equal(t, test.wantBranch, branch.ID, "expected branch")
		})
	}
}

func TestResolve_PoolerDisabled(t *testing.T) {
	resolver := session.NewCNPGBranchResolver("test-namespace", 5432, false)

	t.Run("pooler suffix is not recognized", func(t *testing.T) {
		_, err := resolver.Resolve(context.Background(), "branch1-pooler.example.com", session.EndpointRW)
		require.Error(t, err)
	})

	t.Run("pooler fallback silently degrades to rw", func(t *testing.T) {
		branch, err := resolver.Resolve(context.Background(), "branch1.example.com", session.EndpointPooler)
		require.NoError(t, err)
		require.Equal(t, "branch-branch1-rw.test-namespace.svc.cluster.local:5432", branch.Address)
		require.Equal(t, "branch1", branch.ID)
	})

	t.Run("unknown fallback degrades to rw", func(t *testing.T) {
		branch, err := resolver.Resolve(context.Background(), "branch1.example.com", "bogus")
		require.NoError(t, err)
		require.Equal(t, "branch-branch1-rw.test-namespace.svc.cluster.local:5432", branch.Address)
		require.Equal(t, "branch1", branch.ID)
	})
}

func TestResolve_Error(t *testing.T) {
	resolver := session.NewCNPGBranchResolver("test-namespace", 5432, false)

	tests := map[string]struct {
		serverName string
	}{
		"invalid_hostname": {
			serverName: "invalidhostname",
		},
		"invalid_special_character": {
			serverName: "branch!invalid.com",
		},
		"invalid_empty_string": {
			serverName: "",
		},
		"invalid_endpoint_suffix": {
			serverName: "branch1-invalid.example.com",
		},
		"invalid_empty_with_suffix": {
			serverName: "-ro",
		},
		"invalid_empty_with_suffix_and_domain": {
			serverName: "-ro.example.com",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := resolver.Resolve(context.Background(), test.serverName, session.EndpointRW)
			require.Error(t, err, "expected error for serverName: %s", test.serverName)
		})
	}
}
