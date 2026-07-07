package rpc

import (
	"context"
	"testing"
	"time"

	"xata/gen/protomocks"
	"xata/internal/token"
	"xata/services/auth/keycloak"
	keycloakMocks "xata/services/auth/keycloak/mocks"
	"xata/services/auth/orgs/orgsmock"
	storeMocks "xata/services/auth/store/mocks"

	"github.com/Nerzal/gocloak/v13"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildUserClaims(t *testing.T) {
	const (
		realm        = "test-realm"
		userID       = "user-123"
		userEmail    = "test@example.com"
		defaultOrgID = "default-org"
	)

	awsMarketplace := keycloak.OrganizationMarketplaceProviderAWS
	tests := map[string]struct {
		defaultOrgID string
		kcOrgs       []keycloak.Organization
		want         map[string]token.Organization
	}{
		"default org added when user has no orgs": {
			defaultOrgID: defaultOrgID,
			kcOrgs:       []keycloak.Organization{},
			want: map[string]token.Organization{
				defaultOrgID: {
					ID:     defaultOrgID,
					Status: token.OrgEnabledStatus,
				},
			},
		},
		"default org added alongside existing orgs": {
			defaultOrgID: defaultOrgID,
			kcOrgs: []keycloak.Organization{
				{
					ID:   "other-org",
					Name: "Other Org",
					Status: keycloak.OrganizationStatus{
						BillingStatus: keycloak.OrganizationBillingStatusOK,
					},
				},
			},
			want: map[string]token.Organization{
				"other-org": {
					ID:     "other-org",
					Status: string(keycloak.OrganizationStateEnabled),
				},
				defaultOrgID: {
					ID:     defaultOrgID,
					Status: token.OrgEnabledStatus,
				},
			},
		},
		"existing org not overwritten when it matches default org ID": {
			defaultOrgID: "other-org",
			kcOrgs: []keycloak.Organization{
				{
					ID:   "other-org",
					Name: "Other Org",
					Status: keycloak.OrganizationStatus{
						DisabledByAdmin: true,
						BillingStatus:   keycloak.OrganizationBillingStatusOK,
					},
				},
			},
			want: map[string]token.Organization{
				"other-org": {
					ID:     "other-org",
					Status: string(keycloak.OrganizationStateDisabled),
				},
			},
		},
		"no default org added when defaultOrgID is empty": {
			defaultOrgID: "",
			kcOrgs: []keycloak.Organization{
				{
					ID:   "other-org",
					Name: "Other Org",
					Status: keycloak.OrganizationStatus{
						BillingStatus: keycloak.OrganizationBillingStatusOK,
					},
				},
			},
			want: map[string]token.Organization{
				"other-org": {
					ID:     "other-org",
					Status: string(keycloak.OrganizationStateEnabled),
				},
			},
		},
		"org with CreatedAt preserves timestamp": {
			defaultOrgID: defaultOrgID,
			kcOrgs: func() []keycloak.Organization {
				t := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
				return []keycloak.Organization{
					{
						ID:   "existing-org",
						Name: "Existing Org",
						Status: keycloak.OrganizationStatus{
							BillingStatus: keycloak.OrganizationBillingStatusOK,
							CreatedAt:     &t,
						},
					},
				}
			}(),
			want: map[string]token.Organization{
				"existing-org": {
					ID:        "existing-org",
					Status:    string(keycloak.OrganizationStateEnabled),
					CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				defaultOrgID: {
					ID:     defaultOrgID,
					Status: token.OrgEnabledStatus,
				},
			},
		},
		"org with marketplace preserves marketplace": {
			defaultOrgID: "",
			kcOrgs: []keycloak.Organization{
				{
					ID:          "aws-org",
					Name:        "AWS Org",
					Marketplace: &awsMarketplace,
					Status: keycloak.OrganizationStatus{
						BillingStatus: keycloak.OrganizationBillingStatusOK,
					},
				},
			},
			want: map[string]token.Organization{
				"aws-org": {
					ID:          "aws-org",
					Status:      string(keycloak.OrganizationStateEnabled),
					Marketplace: "aws",
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockKC := keycloakMocks.NewKeyCloak(t)
			mockStore := storeMocks.NewAuthStore(t)
			mockProjects := protomocks.NewProjectsServiceClient(t)
			mockOrgs := orgsmock.NewOrganizations(t)

			mockKC.EXPECT().GetUserRepresentation(context.Background(), realm, userID).
				Return(keycloak.User{ID: userID, Email: userEmail}, nil)
			mockKC.EXPECT().ListOrganizations(context.Background(), realm, userID).
				Return(tc.kcOrgs, nil)

			svc := NewAuthService(mockStore, gocloak.NewClient("http://localhost"), mockKC, mockProjects, mockOrgs, realm, tc.defaultOrgID)
			got, err := svc.buildUserClaims(context.Background(), userID)

			require.NoError(t, err)
			assert.Equal(t, userID, got.ID)
			assert.Equal(t, userEmail, got.Email)
			assert.Equal(t, tc.want, got.Organizations)
		})
	}
}
