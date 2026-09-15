package scheduler_test

import (
	"os"
	"testing"

	"xata/services/projects/scheduler"
	"xata/services/projects/scheduler/strategy"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchedulerReturnsExpectedStrategies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		configFilePath     string
		expectedStrategies map[string]strategy.Interface
	}{
		{
			name:           "no default strategy, should use built-in default",
			configFilePath: "testdata/empty.yaml",
			expectedStrategies: map[string]strategy.Interface{
				"any-region": &strategy.AlwaysPrimary{},
			},
		},
		{
			name:           "explicit default strategy for all regions",
			configFilePath: "testdata/setdefault.yaml",
			expectedStrategies: map[string]strategy.Interface{
				"any-region": &strategy.Random{},
			},
		},
		{
			name:           "explicit strategy for one region",
			configFilePath: "testdata/one-region.yaml",
			expectedStrategies: map[string]strategy.Interface{
				"us-east-1":    &strategy.AlwaysSecondary{},
				"other-region": &strategy.Random{},
			},
		},
		{
			name:           "explicit strategy for two regions",
			configFilePath: "testdata/two-regions.yaml",
			expectedStrategies: map[string]strategy.Interface{
				"us-east-1":    &strategy.AlwaysPrimary{},
				"eu-central-1": &strategy.Random{},
				"other-region": &strategy.AlwaysSecondary{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := os.Open(tt.configFilePath)
			require.NoError(t, err)
			defer f.Close()

			s, err := scheduler.NewScheduler(f)
			require.NoError(t, err)

			for region, expectedStrategy := range tt.expectedStrategies {
				assert.IsType(t, expectedStrategy, s.StrategyForRegion(region))
			}
		})
	}
}

func TestSchedulerErrorCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		configFilePath string
		expectedError  error
	}{
		{
			name:           "invalid config",
			configFilePath: "testdata/unknown-field.yaml",
			expectedError:  scheduler.ErrInvalidConfig,
		},
		{
			name:           "invalid default strategy",
			configFilePath: "testdata/invalid-default.yaml",
			expectedError:  strategy.ErrInvalidStrategy,
		},
		{
			name:           "invalid strategy for a region",
			configFilePath: "testdata/invalid-region-strategy.yaml",
			expectedError:  strategy.ErrInvalidStrategy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := os.Open(tt.configFilePath)
			require.NoError(t, err)
			defer f.Close()

			_, err = scheduler.NewScheduler(f)
			require.Error(t, err)

			assert.ErrorIs(t, err, tt.expectedError)
		})
	}
}
