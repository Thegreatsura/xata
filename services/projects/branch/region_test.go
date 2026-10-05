package branch

import (
	"testing"

	"xata/services/projects/store"

	"github.com/stretchr/testify/require"
)

func TestIsRegionAvailableForMarketplace(t *testing.T) {
	aws := store.Region{Provider: store.ProviderAWS}
	gcp := store.Region{Provider: store.ProviderGCP}
	custom := store.Region{Provider: store.ProviderCustom}

	tests := map[string]struct {
		marketplace string
		region      store.Region
		want        bool
	}{
		"no marketplace uses any region":          {"", custom, true},
		"aws marketplace allows aws region":       {"aws", aws, true},
		"aws marketplace rejects gcp region":      {"aws", gcp, false},
		"aws marketplace rejects custom region":   {"aws", custom, false},
		"vercel marketplace allows aws region":    {"vercel", aws, true},
		"vercel marketplace allows gcp region":    {"vercel", gcp, true},
		"vercel marketplace allows custom region": {"vercel", custom, true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, IsRegionAvailableForMarketplace(tc.marketplace, tc.region))
		})
	}
}
