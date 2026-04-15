package strategy

import (
	"context"
	"errors"

	clustersv1 "xata/gen/proto/clusters/v1"
	"xata/internal/grpc"
	"xata/internal/o11y"
	"xata/services/projects/store"
)

// MaxCapacity is a strategy that selects the cell with the greatest available
// capacity
type MaxCapacity struct {
	CellClientForCell CellClientFactoryFn
}

// CellClientFactoryFn is a function that creates a ClustersServiceClient for a
// given cell
type CellClientFactoryFn func(ctx context.Context, cell store.Cell) (clustersv1.ClustersServiceClient, func() error, error)

// Schedule selects the cell with the greatest available capacity from the
// provided list of cells.
func (m *MaxCapacity) Schedule(ctx context.Context, cells []store.Cell) (*store.Cell, error) {
	if len(cells) == 0 {
		return nil, errors.New("no cells available for scheduling")
	}

	maxAvailableBytes := uint64(0)
	bestCell := &cells[0]

	for _, cell := range cells {
		// Create a gRPC client for the cell
		client, close, err := m.CellClientForCell(ctx, cell)
		if err != nil {
			return nil, err
		}
		defer close()

		// Get the cell utilization metrics
		resp, err := client.GetCellUtilization(ctx, &clustersv1.GetCellUtilizationRequest{})
		if err != nil {
			return nil, err
		}

		if resp.AvailableBytes != nil && resp.GetAvailableBytes() > maxAvailableBytes {
			maxAvailableBytes = resp.GetAvailableBytes()
			bestCell = &cell
		}
	}

	return bestCell, nil
}

// cellClientForCell is the default implementation of CellClientFactoryFn that
// creates a gRPC client for the given cell
func cellClientForCell(ctx context.Context, cell store.Cell) (clustersv1.ClustersServiceClient, func() error, error) {
	o := o11y.Ctx(ctx)
	conn, err := grpc.NewClient(o, cell.ClustersGRPCURL)
	if err != nil {
		return nil, nil, err
	}

	client := clustersv1.NewClustersServiceClient(conn)
	return client, conn.Close, nil
}
