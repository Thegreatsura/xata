package rpc

import (
	"context"
	"testing"
	"time"

	"xata/gen/protomocks"
	"xata/internal/token"
	"xata/services/auth/api/spec"
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

	tests := map[string]struct {
		defaultOrgID string
		kcOrgs       []spec.Organization
		want         map[string]token.Organization
	}{
		"default org added when user has no orgs": {
			defaultOrgID: defaultOrgID,
			kcOrgs:       []spec.Organization{},
			want: map[string]token.Organization{
				defaultOrgID: {
					ID:     defaultOrgID,
					Status: token.OrgEnabledStatus,
				},
			},
		},
		"default org added alongside existing orgs": {
			defaultOrgID: defaultOrgID,
			kcOrgs: []spec.Organization{
				{
					Id:   "other-org",
					Name: "Other Org",
					Status: spec.OrganizationStatus{
						Status: spec.Enabled,
					},
				},
			},
			want: map[string]token.Organization{
				"other-org": {
					ID:     "other-org",
					Status: string(spec.Enabled),
				},
				defaultOrgID: {
					ID:     defaultOrgID,
					Status: token.OrgEnabledStatus,
				},
			},
		},
		"existing org not overwritten when it matches default org ID": {
			defaultOrgID: "other-org",
			kcOrgs: []spec.Organization{
				{
					Id:   "other-org",
					Name: "Other Org",
					Status: spec.OrganizationStatus{
						Status: spec.Disabled,
					},
				},
			},
			want: map[string]token.Organization{
				"other-org": {
					ID:     "other-org",
					Status: string(spec.Disabled),
				},
			},
		},
		"no default org added when defaultOrgID is empty": {
			defaultOrgID: "",
			kcOrgs: []spec.Organization{
				{
					Id:   "other-org",
					Name: "Other Org",
					Status: spec.OrganizationStatus{
						Status: spec.Enabled,
					},
				},
			},
			want: map[string]token.Organization{
				"other-org": {
					ID:     "other-org",
					Status: string(spec.Enabled),
				},
			},
		},
		"org with CreatedAt preserves timestamp": {
			defaultOrgID: defaultOrgID,
			kcOrgs: func() []spec.Organization {
				t := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
				return []spec.Organization{
					{
						Id:   "existing-org",
						Name: "Existing Org",
						Status: spec.OrganizationStatus{
							Status:    spec.Enabled,
							CreatedAt: &t,
						},
					},
				}
			}(),
			want: map[string]token.Organization{
				"existing-org": {
					ID:        "existing-org",
					Status:    string(spec.Enabled),
					CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				defaultOrgID: {
					ID:     defaultOrgID,
					Status: token.OrgEnabledStatus,
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
