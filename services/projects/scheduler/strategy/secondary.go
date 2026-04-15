package strategy

import (
	"context"
	"errors"

	"xata/services/projects/store"
)

// AlwaysSecondary is a scheduler that always selects the first secondary cell.
type AlwaysSecondary struct{}

// Schedule selects the first secondary cell from the provided list of cells or
// returns an error if no secondary cell is found.
func (a *AlwaysSecondary) Schedule(ctx context.Context, cells []store.Cell) (*store.Cell, error) {
	for i := range cells {
		if !cells[i].Primary {
			return &cells[i], nil
		}
	}
	return nil, errors.New("no secondary cell available for scheduling")
}
