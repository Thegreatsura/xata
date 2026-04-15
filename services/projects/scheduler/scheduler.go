package scheduler

import (
	"fmt"
	"io"

	"xata/services/projects/scheduler/strategy"

	"gopkg.in/yaml.v3"
)

// Scheduler resolves which scheduling strategy to use based on its config
type Scheduler struct {
	DefaultStrategy  strategy.Interface
	regionStrategies map[string]strategy.Interface
}

// NewScheduler creates a new scheduler from the provided configuration reader
func NewScheduler(r io.Reader) (*Scheduler, error) {
	var config struct {
		DefaultStrategy strategy.Name            `yaml:"default"`
		Regions         map[string]strategy.Name `yaml:"regions"`
	}

	// Decode YAML configuration
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}

	// Set default strategy if not specified
	if config.DefaultStrategy == "" {
		config.DefaultStrategy = strategy.AlwaysPrimaryStrategyName
	}

	// Create strategies for all regions in the config
	regionStrategies := make(map[string]strategy.Interface)
	for regionID, strategyName := range config.Regions {
		s, err := strategyName.ToStrategy()
		if err != nil {
			return nil, err
		}
		regionStrategies[regionID] = s
	}

	// Create default strategy
	s, err := config.DefaultStrategy.ToStrategy()
	if err != nil {
		return nil, err
	}
	defaultStrategy := s

	return &Scheduler{
		DefaultStrategy:  defaultStrategy,
		regionStrategies: regionStrategies,
	}, nil
}

// StrategyForRegion returns the appropriate scheduling strategy for the given
// region.
func (s *Scheduler) StrategyForRegion(regionID string) strategy.Interface {
	// Check if we have a specific strategy for this region
	if strategy, ok := s.regionStrategies[regionID]; ok {
		return strategy
	}

	// Fall back to default strategy
	return s.DefaultStrategy
}
