package branch

import (
	"context"
	"fmt"

	"xata/services/projects/store"
)

// MarketplaceRegionParams are the inputs to ValidateMarketplaceRegion.
type MarketplaceRegionParams struct {
	OrganizationID string
	Marketplace    string
	Region         string
}

// ValidateMarketplaceRegion checks that the region is available to the
// organization's marketplace, if any. An empty marketplace imposes no
// restriction.
func (s *Service) ValidateMarketplaceRegion(ctx context.Context, params MarketplaceRegionParams) error {
	if params.Marketplace == "" {
		return nil
	}

	region, err := s.store.GetRegion(ctx, params.OrganizationID, params.Region)
	if err != nil {
		return err
	}
	return validateRegionForMarketplace(params.Marketplace, *region)
}

func IsRegionAvailableForMarketplace(marketplace string, region store.Region) bool {
	return marketplace == "" || marketplace == string(region.Provider)
}

func validateRegionForMarketplace(marketplace string, region store.Region) error {
	if IsRegionAvailableForMarketplace(marketplace, region) {
		return nil
	}
	return ErrInvalidRegion{Message: fmt.Sprintf("provider %s is not available for %s marketplace organizations", region.Provider, marketplace)}
}
