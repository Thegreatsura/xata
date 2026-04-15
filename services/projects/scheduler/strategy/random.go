package strategy

import (
	"context"
	"errors"
	"math/rand/v2"

	"xata/services/projects/store"
)

// Random is a scheduler that randomly selects a cell from the available cells.
type Random struct{}

// Schedule randomly selects a cell from the provided list of cells or
// returns an error if no cells are available.
func (a *Random) Schedule(ctx context.Context, cells []store.Cell) (*store.Cell, error) {
	if len(cells) == 0 {
		return nil, errors.New("no cells available for scheduling")
	}

	//nolint:gosec
	index := rand.IntN(len(cells))
	return &cells[index], nil
}
