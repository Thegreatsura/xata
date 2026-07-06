package cells

import (
	"context"
	"errors"
	"fmt"
	"sync"

	clustersv1 "xata/gen/proto/clusters/v1"
	"xata/internal/grpc"
	"xata/internal/o11y"
	"xata/services/projects/store"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//go:generate go run github.com/vektra/mockery/v3 --output cellsmock --outpkg cellsmock --with-expecter --name Cells
//go:generate go run github.com/vektra/mockery/v3 --output cellsmock --outpkg cellsmock --with-expecter --name CellClient

// Cells client for interacting with the clusters service (connect to cells)
type Cells interface {
	GetCellConnection(ctx context.Context, organizationID, cellID string) (CellClient, error)
	// Close tears down all pooled cell connections. Call once on shutdown.
	Close() error
}

// CellClient is a client for the clusters service
type CellClient interface {
	clustersv1.ClustersServiceClient
	Close() error
}

type cellsImpl struct {
	store store.ProjectsStore

	// conns pools one long-lived gRPC connection per cell, keyed by the
	// cell's gRPC URL. gRPC multiplexes concurrent RPCs over a single
	// connection and reconnects automatically, so connections are reused
	// across operations instead of dialed (and torn down) per call.
	mu    sync.Mutex
	conns map[string]*grpc.ClientConnection
}

func New(store store.ProjectsStore) Cells {
	return &cellsImpl{
		store: store,
		conns: map[string]*grpc.ClientConnection{},
	}
}

type cellClientImpl struct {
	clustersv1.ClustersServiceClient
}

func (s *cellsImpl) GetCellConnection(ctx context.Context, organizationID, cellID string) (CellClient, error) {
	cell, err := s.store.GetCell(ctx, organizationID, cellID)
	if err != nil {
		return nil, err
	}

	conn, err := s.getConn(o11y.Ctx(ctx), cell.ClustersGRPCURL)
	if err != nil {
		return nil, err
	}

	return &cellClientImpl{
		ClustersServiceClient: clustersv1.NewClustersServiceClient(conn),
	}, nil
}

// getConn returns the pooled connection for url, dialing and caching one on
// first use. grpc.NewClient is lazy (it does not block on a handshake), so
// holding the lock while creating it is cheap.
func (s *cellsImpl) getConn(o *o11y.O, url string) (*grpc.ClientConnection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if conn, ok := s.conns[url]; ok {
		return conn, nil
	}

	conn, err := grpc.NewClient(o, url)
	if err != nil {
		return nil, err
	}
	s.conns[url] = conn
	return conn, nil
}

// Close is a no-op: the connection is owned and reused by the pool, so
// per-operation callers must not tear it down. The pool is closed via
// cellsImpl.Close on shutdown.
func (c *cellClientImpl) Close() error {
	return nil
}

func (s *cellsImpl) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []error
	for url, conn := range s.conns {
		if err := conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close cell connection [%s]: %w", url, err))
		}
		delete(s.conns, url)
	}
	return errors.Join(errs...)
}

func DeprovisionBranch(ctx context.Context, organizationID string, s store.ProjectsStore, c Cells, b *store.Branch) error {
	client, err := c.GetCellConnection(ctx, organizationID, b.CellID)
	if err != nil {
		return fmt.Errorf("get cell connection: %w", err)
	}
	defer client.Close()

	_, err = client.DeletePostgresCluster(ctx, &clustersv1.DeletePostgresClusterRequest{
		Id: b.ID,
	})
	if err != nil {
		st, _ := status.FromError(err)
		if st.Code() != codes.NotFound {
			return err
		}
		log.Ctx(ctx).Warn().Msgf("branch [%s] not found in Kubernetes, proceeding with deletion", b.ID)
	}

	primaryCell, err := s.GetPrimaryCell(ctx, organizationID, b.Region)
	if err != nil {
		return fmt.Errorf("get primary cell: %w", err)
	}

	// IP filtering is always managed on the primary cell, so we need to clean it up there
	// Get a connection to the primary cell (reuse if branch is already on primary cell)
	var primaryCellClient CellClient
	needsPrimaryCellConnection := primaryCell.ID != b.CellID
	if needsPrimaryCellConnection {
		primaryCellClient, err = c.GetCellConnection(ctx, organizationID, primaryCell.ID)
		if err != nil {
			return fmt.Errorf("get primary cell connection: %w", err)
		}
		defer primaryCellClient.Close()
	} else {
		primaryCellClient = client
	}

	// Clean up IP filtering settings on the primary cell
	_, err = primaryCellClient.DeleteBranchIPFiltering(ctx, &clustersv1.DeleteBranchIPFilteringRequest{
		BranchId: b.ID,
	})
	if err != nil {
		// Log the error but don't fail the deletion if IP filtering cleanup fails
		// The branch is already deleted, so this is best-effort cleanup
		log.Ctx(ctx).Warn().Err(err).Msgf("Failed to delete IP filtering for branch [%s]", b.ID)
	}

	// If the cluster was scheduled on a secondary cell, deregister it from
	// the primary cell
	if needsPrimaryCellConnection {
		_, err = primaryCellClient.DeregisterPostgresCluster(ctx, &clustersv1.DeregisterPostgresClusterRequest{Id: b.ID})
		if err != nil {
			return fmt.Errorf("deregister from primary cell: %w", err)
		}
	}

	return nil
}
