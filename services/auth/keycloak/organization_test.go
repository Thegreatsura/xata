package keycloak

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
