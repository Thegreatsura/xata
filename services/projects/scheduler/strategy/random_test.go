package strategy_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"xata/services/projects/scheduler/strategy"
	"xata/services/projects/store"
)

func TestRandomScheduler(t *testing.T) {
	t.Parallel()

	t.Run("returns error when no cells are available", func(t *testing.T) {
		ctx := context.Background()

		scheduler := strategy.Random{}
		_, err := scheduler.Schedule(ctx, []store.Cell{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no cells available for scheduling")
	})

	t.Run("distributes selections roughly evenly", func(t *testing.T) {
		ctx := context.Background()

		cells := []store.Cell{
			{ID: "cell-1", RegionID: "us-east-1", Primary: true},
			{ID: "cell-2", RegionID: "us-east-1", Primary: false},
			{ID: "cell-3", RegionID: "us-east-1", Primary: false},
			{ID: "cell-4", RegionID: "us-east-1", Primary: false},
		}

		scheduler := strategy.Random{}
		counts := make(map[string]int)
		numIterations := 1000

		// Run scheduler many times and count selections
		for range numIterations {
			result, err := scheduler.Schedule(ctx, cells)
			require.NoError(t, err)
			counts[result.ID]++
		}

		// Each cell should be selected roughly 250 times (25% of 1000)
		// The range 200-300 gives a confidence interval of ~99.97%
		expectedMin := 200
		expectedMax := 300
		for _, count := range counts {
			assert.GreaterOrEqual(t, count, expectedMin)
			assert.LessOrEqual(t, count, expectedMax)
		}

		// Ensure all cells were selected at least once
		assert.Equal(t, len(cells), len(counts))
	})
}
