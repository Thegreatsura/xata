package scheduler

import (
	"fmt"
	"io"

	"xata/services/projects/scheduler/strategy"

	"github.com/goccy/go-yaml"
)

// Scheduler resolves which scheduling strategy to use based on its config
type Scheduler struct {
	DefaultStrategy  strategy.Interface
	regionStrategies map[string]strategy.Interface
}

// config is the scheduler configuration file
type config struct {
	Default strategy.Config            `yaml:"default"`
	Regions map[string]strategy.Config `yaml:"regions"`
}

// NewScheduler creates a new scheduler from the provided configuration reader
func NewScheduler(r io.Reader) (*Scheduler, error) {
	var cfg config
	decoder := yaml.NewDecoder(r, yaml.Strict())
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}

	// Set default strategy if not specified
	defaultStrategy := cfg.Default.Interface
	if defaultStrategy == nil {
		defaultStrategy = &strategy.Random{}
	}

	regionStrategies := make(map[string]strategy.Interface, len(cfg.Regions))
	for regionID, regionCfg := range cfg.Regions {
		regionStrategies[regionID] = regionCfg.Interface
	}

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
