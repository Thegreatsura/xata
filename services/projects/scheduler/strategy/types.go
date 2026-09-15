package strategy

import (
	"context"
	"errors"
	"fmt"

	"xata/services/projects/store"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
)

type Type string

const (
	AlwaysPrimaryStrategyType   Type = "AlwaysPrimary"
	AlwaysSecondaryStrategyType Type = "AlwaysSecondary"
	RandomStrategyType          Type = "Random"
	PinnedStrategyType          Type = "Pinned"
	WeightedStrategyType        Type = "Weighted"
)

var ErrInvalidStrategy = errors.New("invalid strategy")

// Interface defines the interface for a scheduling strategy
type Interface interface {
	Schedule(ctx context.Context, cells []store.Cell) (*store.Cell, error)
}

// validator is implemented by strategies whose parameters need checking
type validator interface {
	Validate() error
}

// registry maps a strategy name to a constructor for its zero value
var registry = map[Type]func() Interface{
	AlwaysPrimaryStrategyType:   func() Interface { return &AlwaysPrimary{} },
	AlwaysSecondaryStrategyType: func() Interface { return &AlwaysSecondary{} },
	RandomStrategyType:          func() Interface { return &Random{} },
	PinnedStrategyType:          func() Interface { return &Pinned{} },
	WeightedStrategyType:        func() Interface { return &Weighted{} },
}

// Config exists to decode a strategy from YAML
type Config struct {
	Interface
}

// UnmarshalYAML implements yaml.NodeUnmarshaler
func (c *Config) UnmarshalYAML(node ast.Node) error {
	mapping, ok := node.(*ast.MappingNode)
	if !ok {
		return fmt.Errorf("line %d: strategy must be a mapping", node.GetToken().Position.Line)
	}

	// Decode the type of strategy
	var head struct {
		Type Type `yaml:"type"`
	}
	if err := yaml.NodeToValue(mapping, &head); err != nil {
		return err
	}
	strategyType := head.Type

	// Construct the desired type of strategy using the registry
	newStrategy, ok := registry[strategyType]
	if !ok {
		return fmt.Errorf("%w - %q", ErrInvalidStrategy, strategyType)
	}
	s := newStrategy()

	// Decode the strategy after removing the `type` field
	if err := yaml.NodeToValue(withoutType(mapping), s, yaml.Strict()); err != nil {
		return fmt.Errorf("%w - %q: %w", ErrInvalidStrategy, strategyType, err)
	}

	// Validate the resulting strategy
	if v, ok := s.(validator); ok {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("%w - %q: %w", ErrInvalidStrategy, strategyType, err)
		}
	}

	c.Interface = s
	return nil
}

// withoutType returns a copy of the mapping with the `type` key removed, so
// that what remains can be decoded strictly into a strategy
func withoutType(n *ast.MappingNode) *ast.MappingNode {
	m := ast.Mapping(n.Start, n.IsFlowStyle)
	for _, v := range n.Values {
		if v.Key.GetToken().Value != "type" {
			m.Values = append(m.Values, v)
		}
	}
	return m
}
