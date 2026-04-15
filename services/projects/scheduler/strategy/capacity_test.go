package strategy_test

import (
	"context"
	"testing"

	clustersv1 "xata/gen/proto/clusters/v1"
	"xata/gen/protomocks"
	"xata/services/projects/scheduler/strategy"
	"xata/services/projects/store"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestMaxCapacityScheduler(t *testing.T) {
	t.Parallel()

	t.Run("returns error when no cells are available", func(t *testing.T) {
		ctx := context.Background()

		scheduler := strategy.MaxCapacity{}
		_, err := scheduler.Schedule(ctx, []store.Cell{})
		require.Error(t, err)
	})

	t.Run("selects the cell with the greatest available capacity", func(t *testing.T) {
		ctx := context.Background()

		cells := []store.Cell{
			{ID: "cell-1", RegionID: "us-east-1", Primary: true},
			{ID: "cell-2", RegionID: "us-east-1", Primary: false},
			{ID: "cell-3", RegionID: "us-east-1", Primary: false},
			{ID: "cell-4", RegionID: "us-east-1", Primary: false},
		}

		scheduler := strategy.MaxCapacity{
			CellClientForCell: func(ctx context.Context, cell store.Cell) (clustersv1.ClustersServiceClient, func() error, error) {
				client := protomocks.NewClustersServiceClient(t)

				client.EXPECT().
					GetCellUtilization(ctx, &clustersv1.GetCellUtilizationRequest{}).
					Return(&clustersv1.GetCellUtilizationResponse{
						AvailableBytes: func() *uint64 {
							switch cell.ID {
							case "cell-1":
								return proto.Uint64(100)
							case "cell-2":
								return proto.Uint64(200)
							case "cell-3":
								return proto.Uint64(800)
							case "cell-4":
								return proto.Uint64(400)
							default:
								return nil
							}
						}(),
					}, nil)

				return client, func() error { return nil }, nil
			},
		}

		selectedCell, err := scheduler.Schedule(ctx, cells)
		require.NoError(t, err)
		require.Equal(t, "cell-3", selectedCell.ID)
	})

	t.Run("ignores cells that do not report available bytes", func(t *testing.T) {
		ctx := context.Background()

		cells := []store.Cell{
			{ID: "cell-1", RegionID: "us-east-1", Primary: true},
			{ID: "cell-2", RegionID: "us-east-1", Primary: false},
			{ID: "cell-3", RegionID: "us-east-1", Primary: false},
			{ID: "cell-4", RegionID: "us-east-1", Primary: false},
		}

		scheduler := strategy.MaxCapacity{
			CellClientForCell: func(ctx context.Context, cell store.Cell) (clustersv1.ClustersServiceClient, func() error, error) {
				client := protomocks.NewClustersServiceClient(t)

				client.EXPECT().
					GetCellUtilization(ctx, &clustersv1.GetCellUtilizationRequest{}).
					Return(&clustersv1.GetCellUtilizationResponse{
						AvailableBytes: func() *uint64 {
							switch cell.ID {
							case "cell-1":
								return nil
							case "cell-2":
								return proto.Uint64(200)
							case "cell-3":
								return nil
							case "cell-4":
								return nil
							default:
								return nil
							}
						}(),
					}, nil)

				return client, func() error { return nil }, nil
			},
		}

		selectedCell, err := scheduler.Schedule(ctx, cells)
		require.NoError(t, err)
		require.Equal(t, "cell-2", selectedCell.ID)
	})

	t.Run("returns the only cell when there is only one", func(t *testing.T) {
		ctx := context.Background()

		cells := []store.Cell{
			{ID: "cell-1", RegionID: "us-east-1", Primary: true},
		}

		scheduler := strategy.MaxCapacity{
			CellClientForCell: func(ctx context.Context, cell store.Cell) (clustersv1.ClustersServiceClient, func() error, error) {
				client := protomocks.NewClustersServiceClient(t)

				client.EXPECT().
					GetCellUtilization(ctx, &clustersv1.GetCellUtilizationRequest{}).
					Return(&clustersv1.GetCellUtilizationResponse{
						AvailableBytes: proto.Uint64(100),
					}, nil)

				return client, func() error { return nil }, nil
			},
		}

		selectedCell, err := scheduler.Schedule(ctx, cells)
		require.NoError(t, err)
		require.Equal(t, "cell-1", selectedCell.ID)
	})

	t.Run("returns the only cell even though it does not report available bytes", func(t *testing.T) {
		ctx := context.Background()

		cells := []store.Cell{
			{ID: "cell-1", RegionID: "us-east-1", Primary: true},
		}

		scheduler := strategy.MaxCapacity{
			CellClientForCell: func(ctx context.Context, cell store.Cell) (clustersv1.ClustersServiceClient, func() error, error) {
				client := protomocks.NewClustersServiceClient(t)

				client.EXPECT().
					GetCellUtilization(ctx, &clustersv1.GetCellUtilizationRequest{}).
					Return(&clustersv1.GetCellUtilizationResponse{
						AvailableBytes: nil,
					}, nil)

				return client, func() error { return nil }, nil
			},
		}

		selectedCell, err := scheduler.Schedule(ctx, cells)
		require.NoError(t, err)
		require.Equal(t, "cell-1", selectedCell.ID)
	})
}
