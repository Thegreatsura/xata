package strategy_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"xata/services/projects/scheduler/strategy"
	"xata/services/projects/store"
)

func TestAlwaysSecondaryScheduler(t *testing.T) {
	t.Parallel()

	t.Run("returns the first secondary cell when one exists", func(t *testing.T) {
		ctx := context.Background()

		testCases := []struct {
			name           string
			cells          []store.Cell
			expectedCellID string
		}{
			{
				name: "cell-2 is secondary",
				cells: []store.Cell{
					{ID: "cell-1", RegionID: "us-east-1", Primary: true},
					{ID: "cell-2", RegionID: "us-east-1", Primary: false},
				},
				expectedCellID: "cell-2",
			},
			{
				name: "cell-1 is secondary",
				cells: []store.Cell{
					{ID: "cell-1", RegionID: "us-east-1", Primary: false},
					{ID: "cell-2", RegionID: "us-east-1", Primary: true},
				},
				expectedCellID: "cell-1",
			},
			{
				name: "multiple secondary cells, returns first",
				cells: []store.Cell{
					{ID: "cell-1", RegionID: "us-east-1", Primary: true},
					{ID: "cell-2", RegionID: "us-east-1", Primary: false},
					{ID: "cell-3", RegionID: "us-east-1", Primary: false},
				},
				expectedCellID: "cell-2",
			},
		}

		scheduler := strategy.AlwaysSecondary{}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				result, err := scheduler.Schedule(ctx, tc.cells)

				require.NoError(t, err)
				assert.False(t, result.Primary)
				assert.Equal(t, tc.expectedCellID, result.ID)
			})
		}
	})

	t.Run("returns error when no secondary cell exists", func(t *testing.T) {
		ctx := context.Background()

		testCases := []struct {
			name  string
			cells []store.Cell
		}{
			{
				name: "only cell is a primary cell",
				cells: []store.Cell{
					{ID: "cell-1", RegionID: "us-east-1", Primary: true},
				},
			},
			{
				name:  "cell list is empty",
				cells: []store.Cell{},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				scheduler := strategy.AlwaysSecondary{}
				_, err := scheduler.Schedule(ctx, tc.cells)
				assert.Error(t, err)
			})
		}
	})
}
