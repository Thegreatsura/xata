package cells

import (
	"context"
	"fmt"

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
}

// CellClient is a client for the clusters service
type CellClient interface {
	clustersv1.ClustersServiceClient
	Close() error
}

type cellsImpl struct {
	store store.ProjectsStore
}

func New(store store.ProjectsStore) Cells {
	return &cellsImpl{
		store: store,
	}
}

type cellClientImpl struct {
	clustersv1.ClustersServiceClient
	conn *grpc.ClientConnection
}

func (s *cellsImpl) GetCellConnection(ctx context.Context, organizationID, cellID string) (CellClient, error) {
	cell, err := s.store.GetCell(ctx, organizationID, cellID)
	if err != nil {
		return nil, err
	}

	o := o11y.Ctx(ctx)
	conn, err := grpc.NewClient(o, cell.ClustersGRPCURL)
	if err != nil {
		return nil, err
	}

	return &cellClientImpl{
		ClustersServiceClient: clustersv1.NewClustersServiceClient(conn),
		conn:                  conn,
	}, nil
}

func (c *cellClientImpl) Close() error {
	return c.conn.Close()
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
