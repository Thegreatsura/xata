package strategy

import (
	"context"

	"xata/services/projects/store"
)

// Random is a scheduler that randomly selects a cell from the available cells.
type Random struct{}

// Schedule randomly selects a cell from the provided list of cells or
// returns an error if no cells are available. It is a weighted draw in which
// every cell has the same weight.
func (a *Random) Schedule(ctx context.Context, cells []store.Cell) (*store.Cell, error) {
	return pick(cells, func(store.Cell) uint { return 1 })
}
