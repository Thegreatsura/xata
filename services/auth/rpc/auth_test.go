package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authv1 "xata/gen/proto/auth/v1"
	"xata/gen/protomocks"
	"xata/internal/api/key"
	"xata/internal/token"
	"xata/services/auth/keycloak"
	keycloakMocks "xata/services/auth/keycloak/mocks"
	"xata/services/auth/orgs/orgsmock"
	storeMocks "xata/services/auth/store/mocks"

	"github.com/Nerzal/gocloak/v13"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

			// buildUserClaims runs these two calls concurrently via errgroup, which
			// derives a child context, so match on any context rather than identity.
			mockKC.EXPECT().GetUserRepresentation(mock.Anything, realm, userID).
				Return(keycloak.User{ID: userID, Email: userEmail}, nil)
			mockKC.EXPECT().ListOrganizations(mock.Anything, realm, userID).
				Return(tc.kcOrgs, nil)

			svc := NewAuthService(mockStore, gocloak.NewClient("http://localhost"), mockKC, mockProjects, mockOrgs, realm, tc.defaultOrgID, false)
			got, err := svc.buildUserClaims(context.Background(), userID)

			require.NoError(t, err)
			assert.Equal(t, userID, got.ID)
			assert.Equal(t, userEmail, got.Email)
			assert.Equal(t, tc.want, got.Organizations)
		})
	}
}

func TestGetOrganization(t *testing.T) {
	tests := map[string]struct {
		includeDeleted bool
		err            error
		wantCode       codes.Code
	}{
		"gets active organization": {},
		"includes deleted organization": {
			includeDeleted: true,
		},
		"maps missing organization to not found": {
			err:      keycloak.ErrOrganizationNotFound{ID: "org-1"},
			wantCode: codes.NotFound,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			keyCloak := keycloakMocks.NewKeyCloak(t)
			organization := keycloak.Organization{ID: "org-1"}
			keyCloak.EXPECT().GetOrganization(mock.Anything, "realm", "org-1", keycloak.GetOrganizationOptions{IncludeDeleted: tc.includeDeleted}).Return(organization, tc.err).Once()
			service := &AuthService{kcRest: keyCloak, realm: "realm"}

			got, err := service.GetOrganization(context.Background(), &authv1.GetOrganizationRequest{
				OrganizationId: "org-1",
				IncludeDeleted: tc.includeDeleted,
			})

			if tc.wantCode != codes.OK {
				require.Equal(t, tc.wantCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, "org-1", got.Organization.Id)
		})
	}
}

func TestGetGithubIdentityProviderTokenRejectsWithoutKeycloak(t *testing.T) {
	apiKey, err := key.NewUserKey()
	require.NoError(t, err)

	tests := map[string]struct {
		token    string
		wantCode codes.Code
	}{
		"empty token": {
			token:    "",
			wantCode: codes.InvalidArgument,
		},
		"api key": {
			token:    string(apiKey),
			wantCode: codes.Unauthenticated,
		},
		"invalid access token": {
			token:    "not-a-jwt",
			wantCode: codes.Unauthenticated,
		},
	}

	// Keycloak certs endpoint always fails, so any token decode fails. The
	// broker token endpoint must never be reached; the KeyCloak mock has no
	// expectations and fails the test on any call.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockKC := keycloakMocks.NewKeyCloak(t)
			mockStore := storeMocks.NewAuthStore(t)
			mockProjects := protomocks.NewProjectsServiceClient(t)
			mockOrgs := orgsmock.NewOrganizations(t)

			svc := NewAuthService(mockStore, gocloak.NewClient(srv.URL), mockKC, mockProjects, mockOrgs, "test-realm", "", false)

			_, err := svc.GetGithubIdentityProviderToken(context.Background(), &authv1.GetGithubIdentityProviderTokenRequest{Token: tc.token})

			require.Error(t, err)
			assert.Equal(t, tc.wantCode, status.Code(err))
		})
	}
}
