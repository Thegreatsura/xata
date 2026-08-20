package billing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	authv1 "xata/gen/proto/auth/v1"
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

func TestCustomerCollectionMethod(t *testing.T) {
	testCases := map[string]struct {
		customer *Customer
		want     CollectionMethod
	}{
		"unknown": {
			customer: &Customer{Organization: &authv1.Organization{BillingCollectionMethod: "unknown"}},
			want:     CollectionMethodUnknown,
		},
		"marketplace": {
			customer: &Customer{Organization: &authv1.Organization{BillingCollectionMethod: "marketplace"}},
			want:     CollectionMethodMarketplace,
		},
		"Stripe payment method": {
			customer: &Customer{Organization: &authv1.Organization{BillingCollectionMethod: "stripe_payment_method"}},
			want:     CollectionMethodStripePaymentMethod,
		},
		"marketplace provider does not override collection method": {
			customer: &Customer{Organization: &authv1.Organization{BillingCollectionMethod: "stripe_payment_method", Marketplace: "aws"}},
			want:     CollectionMethodStripePaymentMethod,
		},
		"bank transfer": {
			customer: &Customer{Organization: &authv1.Organization{BillingCollectionMethod: "bank_transfer"}},
			want:     CollectionMethodBankTransfer,
		},
		"nil customer is unknown": {
			want: CollectionMethodUnknown,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := tc.customer.CollectionMethod()
			require.Equal(t, tc.want, got)
		})
	}
}

func TestCustomerCanCollectPayment(t *testing.T) {
	testCases := map[string]struct {
		customer *Customer
		want     bool
	}{
		"marketplace collection method": {
			customer: &Customer{Organization: &authv1.Organization{BillingCollectionMethod: "marketplace"}},
			want:     true,
		},
		"Stripe collection method with valid payment method": {
			customer: &Customer{Organization: &authv1.Organization{BillingCollectionMethod: "stripe_payment_method"}, HasValidPaymentMethod: true},
			want:     true,
		},
		"Stripe collection method without valid payment method": {
			customer: &Customer{Organization: &authv1.Organization{BillingCollectionMethod: "stripe_payment_method"}},
		},
		"Marketplace provider does not override Stripe collection method": {
			customer: &Customer{Organization: &authv1.Organization{BillingCollectionMethod: "stripe_payment_method", Marketplace: "aws"}},
		},
		"bank transfer collection method without valid Stripe payment method": {
			customer: &Customer{Organization: &authv1.Organization{BillingCollectionMethod: "bank_transfer"}},
			want:     true,
		},
		"bank transfer collection method with valid Stripe payment method": {
			customer: &Customer{Organization: &authv1.Organization{BillingCollectionMethod: "bank_transfer"}, HasValidPaymentMethod: true},
			want:     true,
		},
		"unknown collection method": {
			customer: &Customer{},
		},
		"nil customer": {},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.customer.CanCollectPayment())
		})
	}
}

func TestCustomerCreditCalculations(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	activeExpiry := now.Add(time.Hour)
	lastExpiryWithBalance := now.Add(4 * time.Hour)
	lastExpiry := now.Add(5 * time.Hour)
	credits := []Credit{
		{ID: "active", Amount: 10, Status: CreditStatusActive, EffectiveDate: now.Add(-time.Hour), ExpiryDate: activeExpiry},
		{ID: "active without expiry", Amount: 5, Status: CreditStatusActive},
		{ID: "pending", Amount: 20, Status: CreditStatusPendingPayment, ExpiryDate: lastExpiryWithBalance},
		{ID: "future", Amount: 30, Status: CreditStatusActive, EffectiveDate: now.Add(time.Hour), ExpiryDate: now.Add(3 * time.Hour)},
		{ID: "expired", Amount: 40, Status: CreditStatusActive, ExpiryDate: now.Add(-time.Hour)},
		{ID: "empty balance", Status: CreditStatusActive, ExpiryDate: lastExpiry},
	}

	tests := map[string]struct {
		customer                  *Customer
		wantActive                []Credit
		wantTotalActive           float64
		wantLastExpiry            *time.Time
		wantLastExpiryWithBalance *time.Time
	}{
		"calculates credit details": {
			customer:                  &Customer{Credits: credits},
			wantActive:                []Credit{credits[0], credits[1]},
			wantTotalActive:           15,
			wantLastExpiry:            &lastExpiry,
			wantLastExpiryWithBalance: &lastExpiryWithBalance,
		},
		"empty credits": {
			customer:   &Customer{},
			wantActive: []Credit{},
		},
		"nil customer": {},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.wantActive, tc.customer.ActiveCredits(now))
			require.Equal(t, tc.wantTotalActive, tc.customer.TotalActiveCredits(now))
			require.Equal(t, tc.wantLastExpiry, tc.customer.LastExpiry())
			require.Equal(t, tc.wantLastExpiryWithBalance, tc.customer.LastExpiryWithBalance())
		})
	}
}

func TestLastActiveCreditExpiry(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	earlierExpiry := now.Add(time.Hour)
	laterExpiry := now.Add(2 * time.Hour)

	tests := map[string]struct {
		customer *Customer
		want     *time.Time
	}{
		"returns latest active expiry": {
			customer: &Customer{Credits: []Credit{
				{Amount: 10, Status: CreditStatusActive, EffectiveDate: now.Add(-time.Hour), ExpiryDate: earlierExpiry},
				{Amount: 20, Status: CreditStatusActive, EffectiveDate: now, ExpiryDate: laterExpiry},
			}},
			want: &laterExpiry,
		},
		"ignores credits that are not strictly active": {
			customer: &Customer{Credits: []Credit{
				{Amount: 10, Status: CreditStatusPendingPayment, ExpiryDate: laterExpiry},
				{Amount: 0, Status: CreditStatusActive, ExpiryDate: laterExpiry},
				{Amount: 10, Status: CreditStatusActive, EffectiveDate: now.Add(time.Hour), ExpiryDate: laterExpiry},
				{Amount: 10, Status: CreditStatusActive, ExpiryDate: now},
			}},
		},
		"ignores active credit without expiry": {
			customer: &Customer{Credits: []Credit{{Amount: 10, Status: CreditStatusActive}}},
		},
		"nil customer": {},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.customer.LastActiveCreditExpiry(now)
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
