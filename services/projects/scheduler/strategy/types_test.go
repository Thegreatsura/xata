package strategy_test

import (
	"testing"

	"xata/services/projects/scheduler/strategy"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
)

func TestConfigUnmarshalYAML(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		yaml    string
		want    strategy.Interface
		wantErr error
	}{
		// happy paths
		"Random": {
			yaml: "type: Random",
			want: &strategy.Random{},
		},
		"AlwaysPrimary": {
			yaml: "type: AlwaysPrimary",
			want: &strategy.AlwaysPrimary{},
		},
		"AlwaysSecondary": {
			yaml: "type: AlwaysSecondary",
			want: &strategy.AlwaysSecondary{},
		},
		"Pinned": {
			yaml: "type: Pinned\ncell: cell-1",
			want: &strategy.Pinned{Cell: "cell-1"},
		},
		"Weighted": {
			yaml: "type: Weighted\nweights:\n  cell-1: 90\n  cell-2: 10",
			want: &strategy.Weighted{Weights: map[string]uint{"cell-1": 90, "cell-2": 10}},
		},

		// parameters not accepted by the strategy
		"Pinned with weights": {
			yaml:    "type: Pinned\ncell: cell-1\nweights:\n  cell-1: 1",
			wantErr: strategy.ErrInvalidStrategy,
		},
		"Weighted with cell": {
			yaml:    "type: Weighted\ncell: cell-1\nweights:\n  cell-1: 1",
			wantErr: strategy.ErrInvalidStrategy,
		},
		"Random with cell": {
			yaml:    "type: Random\ncell: cell-1",
			wantErr: strategy.ErrInvalidStrategy,
		},
		"AlwaysPrimary with weights": {
			yaml:    "type: AlwaysPrimary\nweights:\n  cell-1: 1",
			wantErr: strategy.ErrInvalidStrategy,
		},

		// parameter validation
		"Pinned without cell": {
			yaml:    "type: Pinned",
			wantErr: strategy.ErrInvalidStrategy,
		},
		"Weighted without weights": {
			yaml:    "type: Weighted",
			wantErr: strategy.ErrInvalidStrategy,
		},
		"Weighted with all weights zero": {
			yaml:    "type: Weighted\nweights:\n  cell-1: 0",
			wantErr: strategy.ErrInvalidStrategy,
		},
		"Weighted with negative weight": {
			yaml:    "type: Weighted\nweights:\n  cell-1: -1",
			wantErr: strategy.ErrInvalidStrategy,
		},

		"unknown name": {
			yaml:    "type: InvalidStrategy",
			wantErr: strategy.ErrInvalidStrategy,
		},
		"missing type": {
			yaml:    "cell: cell-1",
			wantErr: strategy.ErrInvalidStrategy,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var got strategy.Config
			err := yaml.Unmarshal([]byte(tt.yaml), &got)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got.Interface)
		})
	}

	t.Run("non-mapping is rejected", func(t *testing.T) {
		t.Parallel()

		for _, input := range []string{"Random", "- Random"} {
			var got strategy.Config
			err := yaml.Unmarshal([]byte(input), &got)
			require.ErrorContains(t, err, "strategy must be a mapping", input)
		}
	})
}
