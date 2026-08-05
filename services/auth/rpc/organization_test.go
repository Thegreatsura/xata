package rpc

import (
	"testing"
	"time"

	authv1 "xata/gen/proto/auth/v1"
	"xata/services/auth/keycloak"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestKeycloakOrganizationToProto(t *testing.T) {
	createdAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	deletedAt := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	adminReason := "admin disabled"
	billingReason := "missing payment method"
	marketplace := keycloak.OrganizationMarketplaceProviderAWS

	tests := map[string]struct {
		org  keycloak.Organization
		want *authv1.Organization
	}{
		"enabled organization with optional fields": {
			org: keycloak.Organization{
				ID:          "org-enabled",
				Marketplace: &marketplace,
				Status: keycloak.OrganizationStatus{
					BillingStatus: keycloak.OrganizationBillingStatusOK,
					CreatedAt:     &createdAt,
					UsageTier:     keycloak.OrganizationUsageTierT2,
				},
			},
			want: &authv1.Organization{
				Id:            "org-enabled",
				Status:        string(keycloak.OrganizationStateEnabled),
				BillingStatus: string(keycloak.OrganizationBillingStatusOK),
				CreatedAt:     timestamppb.New(createdAt),
				UsageTier:     string(keycloak.OrganizationUsageTierT2),
				Marketplace:   string(keycloak.OrganizationMarketplaceProviderAWS),
			},
		},
		"soft-deleted organization": {
			org: keycloak.Organization{
				ID: "org-deleted",
				Status: keycloak.OrganizationStatus{
					BillingStatus: keycloak.OrganizationBillingStatusDeletionRequested,
					DeletedAt:     &deletedAt,
					UsageTier:     keycloak.OrganizationUsageTierT1,
				},
			},
			want: &authv1.Organization{
				Id:            "org-deleted",
				Status:        string(keycloak.OrganizationStateDisabled),
				BillingStatus: string(keycloak.OrganizationBillingStatusDeletionRequested),
				DeletedAt:     timestamppb.New(deletedAt),
				UsageTier:     string(keycloak.OrganizationUsageTierT1),
			},
		},
		"disabled organization with reasons": {
			org: keycloak.Organization{
				ID: "org-disabled",
				Status: keycloak.OrganizationStatus{
					DisabledByAdmin: true,
					BillingStatus:   keycloak.OrganizationBillingStatusNoPaymentMethod,
					AdminReason:     &adminReason,
					BillingReason:   &billingReason,
					UsageTier:       keycloak.OrganizationUsageTierT1,
				},
			},
			want: &authv1.Organization{
				Id:                    "org-disabled",
				Status:                string(keycloak.OrganizationStateDisabled),
				DisabledByAdmin:       true,
				DisabledByAdminReason: &adminReason,
				BillingStatus:         string(keycloak.OrganizationBillingStatusNoPaymentMethod),
				BillingReason:         &billingReason,
				UsageTier:             string(keycloak.OrganizationUsageTierT1),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := keycloakOrganizationToProto(tc.org)
			require.True(t, proto.Equal(tc.want, got))
		})
	}
}
