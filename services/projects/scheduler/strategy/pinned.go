package strategy

import (
	"context"
	"errors"

	"xata/services/projects/store"
)

// Pinned is a scheduler that always selects one configured cell
type Pinned struct {
	Cell string `yaml:"cell"`
}

// Validate checks that a cell is configured
func (p *Pinned) Validate() error {
	if p.Cell == "" {
		return errors.New("cell is required")
	}
	return nil
}

// Schedule selects the pinned cell from the provided list, or returns an
// error if it is not present
func (p *Pinned) Schedule(ctx context.Context, cells []store.Cell) (*store.Cell, error) {
	return pick(cells, func(c store.Cell) uint {
		if c.ID == p.Cell {
			return 1
		}
		return 0
	})
}
