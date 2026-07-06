package cells

import (
	"context"
	"testing"

	igrpc "xata/internal/grpc"
	"xata/services/projects/store"
	"xata/services/projects/store/mocks"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/connectivity"
)

// TestGetCellConnectionPoolsByURL verifies connections are reused per gRPC URL
// instead of dialed once per operation.
func TestGetCellConnectionPoolsByURL(t *testing.T) {
	tests := map[string]struct {
		// cellIDs to request in order, each mapped to its gRPC URL.
		requests  []store.Cell
		wantConns int
	}{
		"same cell reused": {
			requests: []store.Cell{
				{ID: "cell-a", ClustersGRPCURL: "cell-a:5002"},
				{ID: "cell-a", ClustersGRPCURL: "cell-a:5002"},
			},
			wantConns: 1,
		},
		"distinct urls get distinct conns": {
			requests: []store.Cell{
				{ID: "cell-a", ClustersGRPCURL: "cell-a:5002"},
				{ID: "cell-b", ClustersGRPCURL: "cell-b:5002"},
			},
			wantConns: 2,
		},
		"same url across cells dedupes": {
			requests: []store.Cell{
				{ID: "cell-a", ClustersGRPCURL: "shared:5002"},
				{ID: "cell-b", ClustersGRPCURL: "shared:5002"},
			},
			wantConns: 1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			st := mocks.NewProjectsStore(t)
			for i := range tc.requests {
				cell := tc.requests[i]
				st.EXPECT().GetCell(mock.Anything, "org-1", cell.ID).Return(&cell, nil).Once()
			}

			c := New(st)
			t.Cleanup(func() { _ = c.Close() })

			for i := range tc.requests {
				client, err := c.GetCellConnection(ctx, "org-1", tc.requests[i].ID)
				require.NoError(t, err)
				require.NotNil(t, client)
			}

			impl, ok := c.(*cellsImpl)
			require.True(t, ok)
			require.Len(t, impl.conns, tc.wantConns)
		})
	}
}

// TestCellClientCloseIsNoop verifies that closing a per-operation client does
// not tear down the pooled connection, so the next operation reuses it.
func TestCellClientCloseIsNoop(t *testing.T) {
	ctx := context.Background()
	st := mocks.NewProjectsStore(t)
	st.EXPECT().GetCell(mock.Anything, "org-1", "cell-a").
		Return(&store.Cell{ID: "cell-a", ClustersGRPCURL: "cell-a:5002"}, nil).Times(2)

	c := New(st)
	t.Cleanup(func() { _ = c.Close() })

	client, err := c.GetCellConnection(ctx, "org-1", "cell-a")
	require.NoError(t, err)

	impl, ok := c.(*cellsImpl)
	require.True(t, ok)
	conn := impl.conns["cell-a:5002"]
	require.NotNil(t, conn)

	// Closing the per-operation client must leave the pooled conn intact.
	require.NoError(t, client.Close())
	require.NotEqual(t, connectivity.Shutdown, conn.GetState())
	require.Len(t, impl.conns, 1)

	// The next operation reuses the same pooled connection.
	client2, err := c.GetCellConnection(ctx, "org-1", "cell-a")
	require.NoError(t, err)
	require.NotNil(t, client2)
	require.Same(t, conn, impl.conns["cell-a:5002"])
}

// TestCellsCloseTearsDownPool verifies the pool closes every cached connection
// on shutdown.
func TestCellsCloseTearsDownPool(t *testing.T) {
	ctx := context.Background()
	st := mocks.NewProjectsStore(t)
	st.EXPECT().GetCell(mock.Anything, "org-1", "cell-a").
		Return(&store.Cell{ID: "cell-a", ClustersGRPCURL: "cell-a:5002"}, nil).Once()
	st.EXPECT().GetCell(mock.Anything, "org-1", "cell-b").
		Return(&store.Cell{ID: "cell-b", ClustersGRPCURL: "cell-b:5002"}, nil).Once()

	c := New(st)

	for _, id := range []string{"cell-a", "cell-b"} {
		_, err := c.GetCellConnection(ctx, "org-1", id)
		require.NoError(t, err)
	}

	impl, ok := c.(*cellsImpl)
	require.True(t, ok)
	conns := []*igrpc.ClientConnection{impl.conns["cell-a:5002"], impl.conns["cell-b:5002"]}
	require.Len(t, impl.conns, 2)

	require.NoError(t, c.Close())
	require.Empty(t, impl.conns)
	for _, conn := range conns {
		require.Equal(t, connectivity.Shutdown, conn.GetState())
	}
}
