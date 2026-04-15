package strategy

import (
	"context"
	"errors"

	"xata/services/projects/store"
)

// AlwaysPrimary is a scheduler that always selects the primary cell.
type AlwaysPrimary struct{}

// Schedule selects the first primary cell from the provided list of cells or
// returns an error if no primary cell is found.
func (a *AlwaysPrimary) Schedule(ctx context.Context, cells []store.Cell) (*store.Cell, error) {
	for i := range cells {
		if cells[i].Primary {
			return &cells[i], nil
		}
	}
	return nil, errors.New("no primary cell available for scheduling")
}
