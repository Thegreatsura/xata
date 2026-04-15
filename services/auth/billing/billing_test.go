package billing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrbCustomerMetadataValues(t *testing.T) {
	tests := map[string]struct {
		metadata OrbCustomerMetadata
		want     map[string]string
	}{
		"empty marketplace returns nil": {
			metadata: OrbCustomerMetadata{},
			want:     nil,
		},
		"non-empty marketplace returns metadata map": {
			metadata: OrbCustomerMetadata{Marketplace: "aws"},
			want:     map[string]string{"marketplace": "aws"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.metadata.Values()
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTotalLifetimeCredits(t *testing.T) {
	tests := map[string]struct {
		credits []Credit
		want    float64
	}{
		"no credits": {
			credits: nil,
			want:    0,
		},
		"single credit": {
			credits: []Credit{{ID: "c1", MaximumInitialBalance: 100}},
			want:    100,
		},
		"multiple credits summed": {
			credits: []Credit{
				{ID: "c1", MaximumInitialBalance: 100},
				{ID: "c2", MaximumInitialBalance: 50.5},
			},
			want: 150.5,
		},
		"credits with zero balance included": {
			credits: []Credit{
				{ID: "c1", MaximumInitialBalance: 100},
				{ID: "c2", MaximumInitialBalance: 0},
			},
			want: 100,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			customer := &Customer{Credits: tc.credits}
			got := customer.TotalLifetimeCredits()
			require.Equal(t, tc.want, got)
		})
	}
}
