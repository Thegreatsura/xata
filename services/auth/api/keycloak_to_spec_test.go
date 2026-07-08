package api

import (
	"testing"
	"time"

	"xata/services/auth/api/spec"
	"xata/services/auth/keycloak"

	"github.com/stretchr/testify/require"
)

func TestToSpecOrganization(t *testing.T) {
	marketplace := keycloak.OrganizationMarketplaceProviderAWS
	marketplaceSpec := spec.OrganizationMarketplaceProvider("aws")
	cases := map[string]struct {
		org  keycloak.Organization
		want spec.Organization
	}{
		"without marketplace": {
			org: keycloak.Organization{ID: "org-1", Name: "Org 1", Status: keycloak.OrganizationStatus{BillingStatus: keycloak.OrganizationBillingStatusOK}},
			want: spec.Organization{Id: "org-1", Name: "Org 1", Status: spec.OrganizationStatus{
				BillingStatus: spec.Ok,
				Status:        spec.Enabled,
			}},
		},
		"with marketplace": {
			org: keycloak.Organization{ID: "org-2", Name: "Org 2", Marketplace: &marketplace, Status: keycloak.OrganizationStatus{BillingStatus: keycloak.OrganizationBillingStatusOK}},
			want: spec.Organization{Id: "org-2", Name: "Org 2", Marketplace: &marketplaceSpec, Status: spec.OrganizationStatus{
				BillingStatus: spec.Ok,
				Status:        spec.Enabled,
			}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, ToSpecOrganization(tc.org))
		})
	}
}

func TestToSpecOrganizations(t *testing.T) {
	got := ToSpecOrganizations([]keycloak.Organization{
		{ID: "org-1", Name: "Org 1", Status: keycloak.OrganizationStatus{BillingStatus: keycloak.OrganizationBillingStatusOK}},
		{ID: "org-2", Name: "Org 2", Status: keycloak.OrganizationStatus{BillingStatus: keycloak.OrganizationBillingStatusNoPaymentMethod}},
	})

	require.Equal(t, []spec.Organization{
		{Id: "org-1", Name: "Org 1", Status: spec.OrganizationStatus{BillingStatus: spec.Ok, Status: spec.Enabled}},
		{Id: "org-2", Name: "Org 2", Status: spec.OrganizationStatus{BillingStatus: spec.NoPaymentMethod, Status: spec.Disabled}},
	}, got)
}

func TestToSpecOrganizationStatus(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	lastUpdated := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	adminReason := "admin"
	billingReason := "billing"
	cases := map[string]struct {
		status keycloak.OrganizationStatus
		want   spec.OrganizationStatus
	}{
		"enabled": {
			status: keycloak.OrganizationStatus{BillingStatus: keycloak.OrganizationBillingStatusOK, UsageTier: keycloak.OrganizationUsageTierT2},
			want:   spec.OrganizationStatus{BillingStatus: spec.Ok, Status: spec.Enabled, UsageTier: spec.T2},
		},
		"disabled with passthrough fields": {
			status: keycloak.OrganizationStatus{
				DisabledByAdmin: true,
				BillingStatus:   keycloak.OrganizationBillingStatusNoPaymentMethod,
				AdminReason:     &adminReason,
				BillingReason:   &billingReason,
				LastUpdated:     lastUpdated,
				CreatedAt:       &createdAt,
				UsageTier:       keycloak.OrganizationUsageTierT1,
			},
			want: spec.OrganizationStatus{
				DisabledByAdmin: true,
				BillingStatus:   spec.NoPaymentMethod,
				AdminReason:     &adminReason,
				BillingReason:   &billingReason,
				LastUpdated:     lastUpdated,
				CreatedAt:       &createdAt,
				UsageTier:       spec.T1,
				Status:          spec.Disabled,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, ToSpecOrganizationStatus(tc.status))
		})
	}
}

func TestToSpecOrganizationMembers(t *testing.T) {
	testCases := map[string]struct {
		members []keycloak.OrganizationMember
		want    []spec.UserWithID
	}{
		"members": {
			members: []keycloak.OrganizationMember{
				{ID: "u1", Email: "one@example.com", Name: "One"},
				{ID: "u2", Email: "two@example.com", Name: "Two"},
			},
			want: []spec.UserWithID{
				{Id: "u1", Email: "one@example.com", Name: "One"},
				{Id: "u2", Email: "two@example.com", Name: "Two"},
			},
		},
		"empty": {
			members: []keycloak.OrganizationMember{},
			want:    []spec.UserWithID{},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := ToSpecOrganizationMembers(tc.members)
			require.Equal(t, tc.want, got)
		})
	}
}
