package orgs

import (
	"context"
	"fmt"
	"testing"

	projectsv1 "xata/gen/proto/projects/v1"
	"xata/gen/protomocks"
	"xata/internal/apitest"
	"xata/services/auth/api"
	"xata/services/auth/keycloak"
	keycloakMocks "xata/services/auth/keycloak/mocks"

	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestUpdateOrganization(t *testing.T) {
	tests := []struct {
		name           string
		organizationID string
		request        UpdateOrganizationOptions
		setupMock      func(mockKc *keycloakMocks.KeyCloak, mockProj *protomocks.ProjectsServiceClient)
		wantErr        bool
		err            error
	}{
		{
			name:           "Unknown organization returns error",
			organizationID: "org-unknown",
			request: UpdateOrganizationOptions{
				DisabledByAdmin: new(true),
			},
			setupMock: func(mockKc *keycloakMocks.KeyCloak, mockProj *protomocks.ProjectsServiceClient) {
				mockKc.EXPECT().GetOrganization(mock.Anything, apitest.TestRealm, "org-unknown", keycloak.GetOrganizationOptions{IncludeDeleted: false}).
					Return(keycloak.Organization{}, api.ErrorNoOrganizationAccess{OrganizationID: "org-unknown"})
			},
			wantErr: true,
			err:     api.ErrorNoOrganizationAccess{OrganizationID: "org-unknown"},
		},
		{
			name:           "Empty request object doesn't invoke any update",
			organizationID: "org-123",
			request:        UpdateOrganizationOptions{},
			setupMock: func(mockKc *keycloakMocks.KeyCloak, mockProj *protomocks.ProjectsServiceClient) {
				mockKc.EXPECT().GetOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.GetOrganizationOptions{IncludeDeleted: false}).
					Return(keycloak.Organization{ID: "org-123"}, nil)
			},
			wantErr: false,
		},
		{
			name:           "disabledByAdmin disables org and triggers project update",
			organizationID: "org-123",
			request: UpdateOrganizationOptions{
				DisabledByAdmin:       new(true),
				DisabledByAdminReason: new("Violation of terms"),
			},
			setupMock: func(mockKc *keycloakMocks.KeyCloak, mockProj *protomocks.ProjectsServiceClient) {
				mockKc.EXPECT().GetOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.GetOrganizationOptions{IncludeDeleted: false}).
					Return(keycloak.Organization{ID: "org-123", Status: keycloak.OrganizationStatus{DisabledByAdmin: false, BillingStatus: keycloak.OrganizationBillingStatusOK}}, nil)

				mockKc.EXPECT().UpdateOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.OrganizationUpdate{
					DisabledByAdmin: new(true),
					AdminReason:     new("Violation of terms"),
				}).Return(keycloak.Organization{
					ID: "org-123",
					Status: keycloak.OrganizationStatus{
						DisabledByAdmin: true,
						BillingStatus:   keycloak.OrganizationBillingStatusOK,
					},
				}, nil)
				mockProj.EXPECT().UpdateOrganizationStatus(mock.Anything,
					&projectsv1.UpdateOrganizationStatusRequest{
						OrganizationId: "org-123",
						Disabled:       true,
					}).
					Return(&projectsv1.UpdateOrganizationStatusResponse{}, nil)
			},
			wantErr: false,
		},
		{
			name:           "billingStatus disables org and triggers project update",
			organizationID: "org-123",
			request: UpdateOrganizationOptions{
				BillingStatus: ptr.To(keycloak.OrganizationBillingStatusNoPaymentMethod),
				BillingReason: new("No payment method on file"),
			},
			setupMock: func(mockKc *keycloakMocks.KeyCloak, mockProj *protomocks.ProjectsServiceClient) {
				mockKc.EXPECT().GetOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.GetOrganizationOptions{IncludeDeleted: false}).
					Return(keycloak.Organization{ID: "org-123", Status: keycloak.OrganizationStatus{DisabledByAdmin: false, BillingStatus: keycloak.OrganizationBillingStatusOK}}, nil)

				mockKc.EXPECT().UpdateOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.OrganizationUpdate{
					BillingStatus: ptr.To(keycloak.OrganizationBillingStatusNoPaymentMethod),
					BillingReason: new("No payment method on file"),
				}).Return(keycloak.Organization{
					ID: "org-123",
					Status: keycloak.OrganizationStatus{
						DisabledByAdmin: false,
						BillingStatus:   keycloak.OrganizationBillingStatusNoPaymentMethod,
					},
				}, nil)
				mockProj.EXPECT().UpdateOrganizationStatus(mock.Anything,
					&projectsv1.UpdateOrganizationStatusRequest{
						OrganizationId: "org-123",
						Disabled:       true,
					}).
					Return(&projectsv1.UpdateOrganizationStatusResponse{}, nil)
			},
			wantErr: false,
		},
		{
			name:           "No status change when updating reason only",
			organizationID: "org-123",
			request: UpdateOrganizationOptions{
				DisabledByAdminReason: new("Updated reason only"),
			},
			setupMock: func(mockKc *keycloakMocks.KeyCloak, mockProj *protomocks.ProjectsServiceClient) {
				mockKc.EXPECT().GetOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.GetOrganizationOptions{IncludeDeleted: false}).
					Return(keycloak.Organization{ID: "org-123", Status: keycloak.OrganizationStatus{DisabledByAdmin: false, BillingStatus: keycloak.OrganizationBillingStatusOK}}, nil)
			},
			wantErr: false,
		},
		{
			name:           "No projects update when general status unchanged",
			organizationID: "org-123",
			request: UpdateOrganizationOptions{
				DisabledByAdmin: new(false),
				BillingStatus:   ptr.To(keycloak.OrganizationBillingStatusNoPaymentMethod),
				BillingReason:   new("No payment method on file"),
			},
			setupMock: func(mockKc *keycloakMocks.KeyCloak, mockProj *protomocks.ProjectsServiceClient) {
				mockKc.EXPECT().GetOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.GetOrganizationOptions{IncludeDeleted: false}).
					Return(keycloak.Organization{ID: "org-123", Status: keycloak.OrganizationStatus{DisabledByAdmin: true, BillingStatus: keycloak.OrganizationBillingStatusOK}}, nil)

				mockKc.EXPECT().UpdateOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.OrganizationUpdate{
					DisabledByAdmin: new(false),
					BillingStatus:   ptr.To(keycloak.OrganizationBillingStatusNoPaymentMethod),
					BillingReason:   new("No payment method on file"),
				}).Return(keycloak.Organization{
					ID: "org-123",
					Status: keycloak.OrganizationStatus{
						DisabledByAdmin: false,
						BillingStatus:   keycloak.OrganizationBillingStatusNoPaymentMethod,
						BillingReason:   new("No payment method on file"),
					},
				}, nil)
			},
			wantErr: false,
		},
		{
			name:           "re-enabling disabled org clears resourcesCleanedAt",
			organizationID: "org-123",
			request: UpdateOrganizationOptions{
				BillingStatus: ptr.To(keycloak.OrganizationBillingStatusOK),
				BillingReason: new("Payment method added"),
			},
			setupMock: func(mockKc *keycloakMocks.KeyCloak, mockProj *protomocks.ProjectsServiceClient) {
				mockKc.EXPECT().GetOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.GetOrganizationOptions{IncludeDeleted: false}).
					Return(keycloak.Organization{
						ID: "org-123",
						Status: keycloak.OrganizationStatus{
							DisabledByAdmin: false,
							BillingStatus:   keycloak.OrganizationBillingStatusNoPaymentMethod,
						},
					}, nil)

				mockKc.EXPECT().UpdateOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.OrganizationUpdate{
					BillingStatus:      ptr.To(keycloak.OrganizationBillingStatusOK),
					BillingReason:      new("Payment method added"),
					ResourcesCleanedAt: new(""),
				}).Return(keycloak.Organization{
					ID: "org-123",
					Status: keycloak.OrganizationStatus{
						DisabledByAdmin: false,
						BillingStatus:   keycloak.OrganizationBillingStatusOK,
					},
				}, nil)

				mockProj.EXPECT().UpdateOrganizationStatus(mock.Anything,
					&projectsv1.UpdateOrganizationStatusRequest{
						OrganizationId: "org-123",
						Disabled:       false,
					}).
					Return(&projectsv1.UpdateOrganizationStatusResponse{}, nil)
			},
			wantErr: false,
		},
		{
			name:           "UsageTier change updates Keycloak but does not trigger projects update",
			organizationID: "org-123",
			request: UpdateOrganizationOptions{
				UsageTier: ptr.To(keycloak.OrganizationUsageTierT2),
			},
			setupMock: func(mockKc *keycloakMocks.KeyCloak, mockProj *protomocks.ProjectsServiceClient) {
				mockKc.EXPECT().GetOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.GetOrganizationOptions{IncludeDeleted: false}).
					Return(keycloak.Organization{
						ID: "org-123",
						Status: keycloak.OrganizationStatus{
							DisabledByAdmin: false,
							BillingStatus:   keycloak.OrganizationBillingStatusOK,
							UsageTier:       keycloak.OrganizationUsageTierT1,
						},
					}, nil)

				mockKc.EXPECT().UpdateOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.OrganizationUpdate{
					UsageTier: ptr.To(keycloak.OrganizationUsageTierT2),
				}).Return(keycloak.Organization{
					ID: "org-123",
					Status: keycloak.OrganizationStatus{
						DisabledByAdmin: false,
						BillingStatus:   keycloak.OrganizationBillingStatusOK,
						UsageTier:       keycloak.OrganizationUsageTierT2,
					},
				}, nil)
				// mockProj is intentionally not called
			},
			wantErr: false,
		},
		{
			name:           "UsageTier unchanged does not update Keycloak",
			organizationID: "org-123",
			request: UpdateOrganizationOptions{
				UsageTier: ptr.To(keycloak.OrganizationUsageTierT1),
			},
			setupMock: func(mockKc *keycloakMocks.KeyCloak, mockProj *protomocks.ProjectsServiceClient) {
				mockKc.EXPECT().GetOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.GetOrganizationOptions{IncludeDeleted: false}).
					Return(keycloak.Organization{
						ID: "org-123",
						Status: keycloak.OrganizationStatus{
							DisabledByAdmin: false,
							BillingStatus:   keycloak.OrganizationBillingStatusOK,
							UsageTier:       keycloak.OrganizationUsageTierT1,
						},
					}, nil)
				// mockKc.UpdateOrganization and mockProj are intentionally not called
			},
			wantErr: false,
		},
		{
			name:           "projects update retries and eventually fails",
			organizationID: "org-123",
			request:        UpdateOrganizationOptions{DisabledByAdmin: new(true)},
			setupMock: func(mockKc *keycloakMocks.KeyCloak, mockProj *protomocks.ProjectsServiceClient) {
				mockKc.EXPECT().GetOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.GetOrganizationOptions{IncludeDeleted: false}).Return(keycloak.Organization{
					ID:     "org-123",
					Status: keycloak.OrganizationStatus{DisabledByAdmin: false, BillingStatus: keycloak.OrganizationBillingStatusOK},
				}, nil)

				mockKc.EXPECT().UpdateOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.OrganizationUpdate{
					DisabledByAdmin: new(true),
				}).Return(keycloak.Organization{
					ID:     "org-123",
					Status: keycloak.OrganizationStatus{DisabledByAdmin: true, BillingStatus: keycloak.OrganizationBillingStatusOK},
				}, nil)

				mockProj.EXPECT().
					UpdateOrganizationStatus(mock.Anything, mock.Anything).
					Return((*projectsv1.UpdateOrganizationStatusResponse)(nil), fmt.Errorf("boom")).
					Times(int(projectsMaxRetries) + 1) // total attempts
			},
			wantErr: true,
		},
		{
			name:           "projects update retries and then succeeds",
			organizationID: "org-123",
			request: UpdateOrganizationOptions{
				DisabledByAdmin: new(true),
			},
			setupMock: func(mockKc *keycloakMocks.KeyCloak, mockProj *protomocks.ProjectsServiceClient) {
				mockKc.EXPECT().
					GetOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.GetOrganizationOptions{IncludeDeleted: false}).
					Return(keycloak.Organization{
						ID: "org-123",
						Status: keycloak.OrganizationStatus{
							DisabledByAdmin: false,
							BillingStatus:   keycloak.OrganizationBillingStatusOK,
						},
					}, nil).
					Once()

				mockKc.EXPECT().
					UpdateOrganization(mock.Anything, apitest.TestRealm, "org-123", keycloak.OrganizationUpdate{
						DisabledByAdmin: new(true),
					}).
					Return(keycloak.Organization{
						ID: "org-123",
						Status: keycloak.OrganizationStatus{
							DisabledByAdmin: true,
							BillingStatus:   keycloak.OrganizationBillingStatusOK,
						},
					}, nil).
					Once()

				// Fail twice, then succeed.
				mockProj.EXPECT().
					UpdateOrganizationStatus(mock.Anything, &projectsv1.UpdateOrganizationStatusRequest{
						OrganizationId: "org-123",
						Disabled:       true,
					}).
					Return((*projectsv1.UpdateOrganizationStatusResponse)(nil), assert.AnError).
					Times(2)

				mockProj.EXPECT().
					UpdateOrganizationStatus(mock.Anything, &projectsv1.UpdateOrganizationStatusRequest{
						OrganizationId: "org-123",
						Disabled:       true,
					}).
					Return(&projectsv1.UpdateOrganizationStatusResponse{}, nil).
					Once()
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProj := protomocks.NewProjectsServiceClient(t)
			mockKc := keycloakMocks.NewKeyCloak(t)
			if tt.setupMock != nil {
				tt.setupMock(mockKc, mockProj)
			}

			service, ok := NewOrganizations(apitest.TestRealm, mockKc, mockProj).(*orgsService)
			require.True(t, ok)
			service.newBackoff = func() backoff.BackOff { return backoff.NewConstantBackOff(0) }

			resp, err := service.UpdateOrganization(context.Background(), tt.organizationID, tt.request)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
			}
		})
	}
}
