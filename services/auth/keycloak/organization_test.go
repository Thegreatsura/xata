package keycloak

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationForceBillingStatusOkFromAttributes(t *testing.T) {
	testCases := map[string]struct {
		attributes map[string][]string
		want       bool
	}{
		"missing":          {},
		"true":             {attributes: map[string][]string{OrganizationForceBillingStatusOkKey: {"true"}}, want: true},
		"case insensitive": {attributes: map[string][]string{OrganizationForceBillingStatusOkKey: {"TRUE"}}, want: true},
		"false":            {attributes: map[string][]string{OrganizationForceBillingStatusOkKey: {"false"}}},
		"empty list":       {attributes: map[string][]string{OrganizationForceBillingStatusOkKey: {}}},
		"empty value":      {attributes: map[string][]string{OrganizationForceBillingStatusOkKey: {""}}},
		"invalid":          {attributes: map[string][]string{OrganizationForceBillingStatusOkKey: {"invalid"}}},
		"first value wins": {attributes: map[string][]string{OrganizationForceBillingStatusOkKey: {"false", "true"}}},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := OrganizationFromAttributes("org-1", tc.attributes)
			require.Equal(t, tc.want, got.ForceBillingStatusOk)
		})
	}
}

func TestOrganizationBillingCollectionMethodFromAttributes(t *testing.T) {
	testCases := map[string]struct {
		attributes map[string][]string
		want       OrganizationBillingCollectionMethod
	}{
		"stripe payment method": {
			attributes: map[string][]string{OrganizationBillingCollectionMethodKey: {string(OrganizationBillingCollectionMethodStripePaymentMethod)}},
			want:       OrganizationBillingCollectionMethodStripePaymentMethod,
		},
		"marketplace": {
			attributes: map[string][]string{OrganizationBillingCollectionMethodKey: {string(OrganizationBillingCollectionMethodMarketplace)}},
			want:       OrganizationBillingCollectionMethodMarketplace,
		},
		"bank transfer": {
			attributes: map[string][]string{OrganizationBillingCollectionMethodKey: {string(OrganizationBillingCollectionMethodBankTransfer)}},
			want:       OrganizationBillingCollectionMethodBankTransfer,
		},
		"missing is unknown": {
			want: OrganizationBillingCollectionMethodUnknown,
		},
		"blank is unknown": {
			attributes: map[string][]string{OrganizationBillingCollectionMethodKey: {" "}},
			want:       OrganizationBillingCollectionMethodUnknown,
		},
		"invalid is unknown": {
			attributes: map[string][]string{OrganizationBillingCollectionMethodKey: {"invalid"}},
			want:       OrganizationBillingCollectionMethodUnknown,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := OrganizationFromAttributes("org-1", tc.attributes)
			assert.Equal(t, tc.want, got.BillingCollectionMethod)
		})
	}
}

func TestOrganizationBillingCollectionMethodValid(t *testing.T) {
	testCases := map[string]struct {
		method OrganizationBillingCollectionMethod
		want   bool
	}{
		"stripe payment method": {method: OrganizationBillingCollectionMethodStripePaymentMethod, want: true},
		"marketplace":           {method: OrganizationBillingCollectionMethodMarketplace, want: true},
		"bank transfer":         {method: OrganizationBillingCollectionMethodBankTransfer, want: true},
		"unknown":               {method: OrganizationBillingCollectionMethodUnknown},
		"empty":                 {},
		"unsupported":           {method: OrganizationBillingCollectionMethod("unsupported")},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.method.Valid())
		})
	}
}

func TestOrganizationStatus_State(t *testing.T) {
	cases := map[string]struct {
		status OrganizationStatus
		want   OrganizationState
	}{
		"enabled when billing ok and not admin disabled": {
			status: OrganizationStatus{BillingStatus: OrganizationBillingStatusOK},
			want:   OrganizationStateEnabled,
		},
		"disabled by admin": {
			status: OrganizationStatus{DisabledByAdmin: true, BillingStatus: OrganizationBillingStatusOK},
			want:   OrganizationStateDisabled,
		},
		"disabled by billing": {
			status: OrganizationStatus{BillingStatus: OrganizationBillingStatusNoPaymentMethod},
			want:   OrganizationStateDisabled,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.status.EffectiveState())
		})
	}
}
