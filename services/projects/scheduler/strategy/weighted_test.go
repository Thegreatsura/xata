package strategy_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"xata/services/projects/scheduler/strategy"
	"xata/services/projects/store"
)

func TestWeightedScheduler(t *testing.T) {
	t.Parallel()

	t.Run("only selects cells with a positive weight", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		cells := []store.Cell{
			{ID: "cell-1", RegionID: "us-east-1"}, // not in weights
			{ID: "cell-2", RegionID: "us-east-1"},
			{ID: "cell-3", RegionID: "us-east-1"}, // weight zero
		}
		s := &strategy.Weighted{Weights: map[string]uint{
			"cell-2": 1,
			"cell-3": 0,
			"cell-4": 5, // configured but not in the cell list
		}}

		for range 100 {
			got, err := s.Schedule(ctx, cells)
			require.NoError(t, err)
			require.Equal(t, "cell-2", got.ID)
		}
	})

	t.Run("distributes selections in proportion to weight", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		cells := []store.Cell{
			{ID: "cell-1", RegionID: "us-east-1"},
			{ID: "cell-2", RegionID: "us-east-1"},
		}
		s := &strategy.Weighted{Weights: map[string]uint{
			"cell-1": 90,
			"cell-2": 10,
		}}

		counts := make(map[string]int)
		numIterations := 1000
		for range numIterations {
			got, err := s.Schedule(ctx, cells)
			require.NoError(t, err)
			counts[got.ID]++
		}

		// cell-2 should be selected roughly 100 times (10% of 1000). The range
		// 60-140 is ~4 standard deviations, a confidence interval of ~99.99%
		require.GreaterOrEqual(t, counts["cell-2"], 60)
		require.LessOrEqual(t, counts["cell-2"], 140)
		require.Equal(t, numIterations, counts["cell-1"]+counts["cell-2"])
	})

	t.Run("returns error when no cell has a positive weight", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		s := &strategy.Weighted{Weights: map[string]uint{
			"cell-3": 1, // not in any of the cell lists below
			"cell-2": 0,
		}}

		tests := map[string][]store.Cell{
			"empty cell list": {},
			"cell not in weights": {
				{ID: "cell-1", RegionID: "us-east-1"},
			},
			"cell with weight zero": {
				{ID: "cell-2", RegionID: "us-east-1"},
			},
		}

		for name, cells := range tests {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				_, err := s.Schedule(ctx, cells)
				require.ErrorContains(t, err, "no cells available for scheduling")
			})
		}
	})
}

func TestWeightedValidate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		weights map[string]uint
		wantErr bool
	}{
		"nil weights":               {weights: nil, wantErr: true},
		"empty weights":             {weights: map[string]uint{}, wantErr: true},
		"all weights zero":          {weights: map[string]uint{"cell-1": 0, "cell-2": 0}, wantErr: true},
		"one positive weight":       {weights: map[string]uint{"cell-1": 0, "cell-2": 1}},
		"multiple positive weights": {weights: map[string]uint{"cell-1": 90, "cell-2": 10}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := &strategy.Weighted{Weights: tt.weights}
			err := s.Validate()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
