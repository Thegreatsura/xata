package strategy

import (
	"context"
	"errors"
	"math/rand/v2"

	"xata/services/projects/store"
)

// Weighted is a scheduler that selects a cell at random in proportion to the
// weights configured for it. Cells with no configured weight, or a weight of
// zero, are never selected, so the weights also act as an allow-list
type Weighted struct {
	Weights map[string]uint `yaml:"weights"`
}

// Validate checks that at least one cell has a positive weight
func (w *Weighted) Validate() error {
	for _, weight := range w.Weights {
		if weight > 0 {
			return nil
		}
	}
	return errors.New("at least one cell must have a positive weight")
}

// Schedule selects a cell from the provided list in proportion to its
// configured weight
func (w *Weighted) Schedule(ctx context.Context, cells []store.Cell) (*store.Cell, error) {
	return pick(cells, func(c store.Cell) uint { return w.Weights[c.ID] })
}

// pick draws one cell at random, each with probability proportional to
// weightOf(cell). Cells with weight zero are never drawn.
func pick(cells []store.Cell, weightOf func(store.Cell) uint) (*store.Cell, error) {
	var total uint
	for _, c := range cells {
		total += weightOf(c)
	}
	if total == 0 {
		return nil, errors.New("no cells available for scheduling")
	}

	//nolint:gosec
	r := rand.UintN(total)
	for i := range cells {
		w := weightOf(cells[i])
		if r < w {
			return &cells[i], nil
		}
		r -= w
	}

	// Unreachable: r < total and the weights sum to total.
	return nil, errors.New("no cells available for scheduling")
}
