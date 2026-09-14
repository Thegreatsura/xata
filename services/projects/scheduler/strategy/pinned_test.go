package strategy_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"xata/services/projects/scheduler/strategy"
	"xata/services/projects/store"
)

func TestPinnedScheduler(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := &strategy.Pinned{Cell: "cell-2"}

	t.Run("always returns the pinned cell", func(t *testing.T) {
		t.Parallel()

		tests := map[string][]store.Cell{
			"pinned cell first": {
				{ID: "cell-2", RegionID: "us-east-1"},
				{ID: "cell-1", RegionID: "us-east-1"},
			},
			"pinned cell last": {
				{ID: "cell-1", RegionID: "us-east-1"},
				{ID: "cell-3", RegionID: "us-east-1"},
				{ID: "cell-2", RegionID: "us-east-1"},
			},
			"pinned cell only": {
				{ID: "cell-2", RegionID: "us-east-1"},
			},
		}

		for name, cells := range tests {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				for range 10 {
					got, err := s.Schedule(ctx, cells)
					require.NoError(t, err)
					require.Equal(t, "cell-2", got.ID)
				}
			})
		}
	})

	t.Run("returns error when the pinned cell is not available", func(t *testing.T) {
		t.Parallel()

		cells := []store.Cell{
			{ID: "cell-1", RegionID: "us-east-1"},
			{ID: "cell-3", RegionID: "us-east-1"},
		}
		_, err := s.Schedule(ctx, cells)
		require.ErrorContains(t, err, "no cells available for scheduling")
	})
}

func TestPinnedValidate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cell    string
		wantErr bool
	}{
		"empty cell": {cell: "", wantErr: true},
		"cell set":   {cell: "cell-1"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := &strategy.Pinned{Cell: tt.cell}
			err := s.Validate()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
