package rpc

import (
	"context"
	"errors"
	"fmt"
	"testing"

	clustersv1 "xata/gen/proto/clusters/v1"
	"xata/internal/apitest"
	"xata/services/projects/cells/cellsmock"

	projectsv1 "xata/gen/proto/projects/v1"
	"xata/services/projects/store"
	"xata/services/projects/store/mocks"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCreateRegion(t *testing.T) {
	tests := []struct {
		name          string
		request       *projectsv1.CreateRegionRequest
		expectedFlags store.RegionFlags
		setupMock     func(*mocks.ProjectsStore)
	}{
		{
			name: "create public region with backups enabled",
			request: &projectsv1.CreateRegionRequest{
				Id:             "us-west-1",
				PublicAccess:   true,
				BackupsEnabled: true,
				Hostport:       "us-west-1.example.com",
				Provider:       "aws",
			},
			expectedFlags: store.RegionFlags{
				PublicAccess:   true,
				BackupsEnabled: true,
				Provider:       store.ProviderAWS,
			},
			setupMock: func(mockStore *mocks.ProjectsStore) {
				mockStore.EXPECT().CreateRegion(
					mock.Anything,
					"us-west-1",
					store.RegionFlags{PublicAccess: true, BackupsEnabled: true, Provider: store.ProviderAWS},
					"us-west-1.example.com",
				).Return(&store.Region{}, nil)
			},
		},
		{
			name: "create private region with backups disabled",
			request: &projectsv1.CreateRegionRequest{
				Id:             "eu-central-1",
				PublicAccess:   false,
				BackupsEnabled: false,
				Hostport:       "eu-central-1.example.com",
				Provider:       "aws",
			},
			expectedFlags: store.RegionFlags{
				PublicAccess:   false,
				BackupsEnabled: false,
				Provider:       store.ProviderAWS,
			},
			setupMock: func(mockStore *mocks.ProjectsStore) {
				mockStore.EXPECT().CreateRegion(
					mock.Anything,
					"eu-central-1",
					store.RegionFlags{PublicAccess: false, BackupsEnabled: false, Provider: store.ProviderAWS},
					"eu-central-1.example.com",
				).Return(&store.Region{}, nil)
			},
		},
		{
			name: "create region with explicit provider",
			request: &projectsv1.CreateRegionRequest{
				Id:             "europe-west4",
				PublicAccess:   true,
				BackupsEnabled: true,
				Hostport:       "europe-west4.example.com",
				Provider:       "gcp",
			},
			expectedFlags: store.RegionFlags{
				PublicAccess:   true,
				BackupsEnabled: true,
				Provider:       store.ProviderGCP,
			},
			setupMock: func(mockStore *mocks.ProjectsStore) {
				mockStore.EXPECT().CreateRegion(
					mock.Anything,
					"europe-west4",
					store.RegionFlags{PublicAccess: true, BackupsEnabled: true, Provider: store.ProviderGCP},
					"europe-west4.example.com",
				).Return(&store.Region{}, nil)
			},
		},
		{
			name: "create organization region with mixed flags",
			request: &projectsv1.CreateRegionRequest{
				Id:             "ap-south-1",
				PublicAccess:   true,
				BackupsEnabled: false,
				Hostport:       "ap-south-1.example.com",
				OrganizationId: new("org-123"),
				Provider:       "aws",
			},
			expectedFlags: store.RegionFlags{
				PublicAccess:   true,
				BackupsEnabled: false,
				Provider:       store.ProviderAWS,
			},
			setupMock: func(mockStore *mocks.ProjectsStore) {
				mockStore.EXPECT().CreateOrganizationRegion(
					mock.Anything,
					"org-123",
					"ap-south-1",
					store.RegionFlags{PublicAccess: true, BackupsEnabled: false, Provider: store.ProviderAWS},
					"ap-south-1.example.com",
				).Return(&store.Region{}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := mocks.NewProjectsStore(t)
			tt.setupMock(mockStore)

			service := NewProjectsService(mockStore, nil)

			resp, err := service.CreateRegion(context.Background(), tt.request)

			require.NoError(t, err)
			require.NotNil(t, resp)
		})
	}
}

func TestCreateRegionInvalidProvider(t *testing.T) {
	for name, provider := range map[string]string{
		"unknown provider": "digitalocean",
		"empty provider":   "",
	} {
		t.Run(name, func(t *testing.T) {
			mockStore := mocks.NewProjectsStore(t)
			service := NewProjectsService(mockStore, nil)

			_, err := service.CreateRegion(context.Background(), &projectsv1.CreateRegionRequest{
				Id:       "us-west-1",
				Provider: provider,
			})

			require.Error(t, err)
			require.ErrorContains(t, err, "unknown provider")
		})
	}
}

func TestListRegions(t *testing.T) {
	mockStore := mocks.NewProjectsStore(t)

	expectedRegions := []store.Region{
		{
			ID:             "us-east-1",
			PublicAccess:   true,
			BackupsEnabled: true,
			Provider:       store.ProviderAWS,
			OrganizationID: nil,
		},
		{
			ID:             "eu-west-1",
			PublicAccess:   false,
			BackupsEnabled: false,
			Provider:       store.ProviderGCP,
			OrganizationID: new("org-123"),
		},
	}

	mockStore.EXPECT().ListAllRegions(mock.Anything).Return(expectedRegions, nil)

	service := NewProjectsService(mockStore, nil)

	resp, err := service.ListRegions(context.Background(), &projectsv1.ListRegionsRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Regions, 2)

	// Verify first region
	require.Equal(t, "us-east-1", resp.Regions[0].Id)
	require.Equal(t, true, resp.Regions[0].PublicAccess)
	require.Equal(t, true, resp.Regions[0].BackupsEnabled)
	require.Equal(t, "aws", resp.Regions[0].Provider)
	require.Nil(t, resp.Regions[0].OrganizationId)

	// Verify second region
	require.Equal(t, "eu-west-1", resp.Regions[1].Id)
	require.Equal(t, false, resp.Regions[1].PublicAccess)
	require.Equal(t, false, resp.Regions[1].BackupsEnabled)
	require.Equal(t, "gcp", resp.Regions[1].Provider)
	require.Equal(t, "org-123", *resp.Regions[1].OrganizationId)
}

func TestDeleteProjectsInOrg(t *testing.T) {
	ctx := context.Background()
	orgID := apitest.TestOrganization

	tests := map[string]struct {
		setupMock       func(*mocks.ProjectsStore, *cellsmock.Cells)
		wantProjectsDel int32
		wantBranchesDel int32
		wantErrors      []string
		wantErr         bool
	}{
		"no projects": {
			setupMock: func(mockStore *mocks.ProjectsStore, mockCells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, orgID).Return(nil, nil)
			},
		},
		"organization status delete failure is reported": {
			setupMock: func(mockStore *mocks.ProjectsStore, mockCells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, orgID).Return(nil, nil)
				mockStore.EXPECT().DeleteOrganizationStatus(mock.Anything, orgID).Return(errTest).Once()
			},
			wantErrors: []string{"delete organization status: test error"},
		},
		"single project with single branch on primary cell": {
			setupMock: func(mockStore *mocks.ProjectsStore, mockCells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, orgID).Return([]store.Project{{ID: "proj-1"}}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, orgID, "proj-1").Return([]store.Branch{
					{ID: "branch-1", CellID: "cell-1", Region: "us-east-1"},
				}, nil)

				cellClient := cellsmock.NewCellClient(t)
				mockCells.EXPECT().GetCellConnection(mock.Anything, orgID, "cell-1").Return(cellClient, nil)
				cellClient.EXPECT().DeletePostgresCluster(mock.Anything, &clustersv1.DeletePostgresClusterRequest{Id: "branch-1"}).Return(&clustersv1.DeletePostgresClusterResponse{}, nil)
				cellClient.EXPECT().DeleteBranchIPFiltering(mock.Anything, &clustersv1.DeleteBranchIPFilteringRequest{BranchId: "branch-1"}).Return(&clustersv1.DeleteBranchIPFilteringResponse{}, nil)
				cellClient.EXPECT().Close().Return(nil)
				mockStore.EXPECT().GetPrimaryCell(mock.Anything, orgID, "us-east-1").Return(&store.Cell{ID: "cell-1"}, nil)

				mockStore.EXPECT().DeleteBranch(mock.Anything, orgID, "proj-1", "branch-1", mock.AnythingOfType("func(*store.Branch) error")).
					Run(func(_ context.Context, _, _, _ string, fn func(*store.Branch) error) {
						_ = fn(&store.Branch{ID: "branch-1", CellID: "cell-1", Region: "us-east-1"})
					}).Return(nil)
				mockStore.EXPECT().DeleteProject(mock.Anything, orgID, "proj-1").Return(nil)
			},
			wantProjectsDel: 1,
			wantBranchesDel: 1,
		},
		"branch on non-primary cell triggers deregister": {
			setupMock: func(mockStore *mocks.ProjectsStore, mockCells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, orgID).Return([]store.Project{{ID: "proj-1"}}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, orgID, "proj-1").Return([]store.Branch{
					{ID: "branch-1", CellID: "cell-2", Region: "us-east-1"},
				}, nil)

				cellClient := cellsmock.NewCellClient(t)
				mockCells.EXPECT().GetCellConnection(mock.Anything, orgID, "cell-2").Return(cellClient, nil)
				cellClient.EXPECT().DeletePostgresCluster(mock.Anything, &clustersv1.DeletePostgresClusterRequest{Id: "branch-1"}).Return(&clustersv1.DeletePostgresClusterResponse{}, nil)
				cellClient.EXPECT().Close().Return(nil)

				mockStore.EXPECT().GetPrimaryCell(mock.Anything, orgID, "us-east-1").Return(&store.Cell{ID: "cell-1"}, nil)

				primaryClient := cellsmock.NewCellClient(t)
				mockCells.EXPECT().GetCellConnection(mock.Anything, orgID, "cell-1").Return(primaryClient, nil)
				primaryClient.EXPECT().DeleteBranchIPFiltering(mock.Anything, &clustersv1.DeleteBranchIPFilteringRequest{BranchId: "branch-1"}).Return(&clustersv1.DeleteBranchIPFilteringResponse{}, nil)
				primaryClient.EXPECT().DeregisterPostgresCluster(mock.Anything, &clustersv1.DeregisterPostgresClusterRequest{Id: "branch-1"}).Return(&clustersv1.DeregisterPostgresClusterResponse{}, nil)
				primaryClient.EXPECT().Close().Return(nil)

				mockStore.EXPECT().DeleteBranch(mock.Anything, orgID, "proj-1", "branch-1", mock.AnythingOfType("func(*store.Branch) error")).
					Run(func(_ context.Context, _, _, _ string, fn func(*store.Branch) error) {
						_ = fn(&store.Branch{ID: "branch-1", CellID: "cell-2", Region: "us-east-1"})
					}).Return(nil)
				mockStore.EXPECT().DeleteProject(mock.Anything, orgID, "proj-1").Return(nil)
			},
			wantProjectsDel: 1,
			wantBranchesDel: 1,
		},
		"multiple projects and branches": {
			setupMock: func(mockStore *mocks.ProjectsStore, mockCells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, orgID).Return([]store.Project{
					{ID: "proj-1"},
					{ID: "proj-2"},
				}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, orgID, "proj-1").Return([]store.Branch{
					{ID: "branch-1", CellID: "cell-1", Region: "us-east-1"},
					{ID: "branch-2", CellID: "cell-1", Region: "us-east-1"},
				}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, orgID, "proj-2").Return([]store.Branch{
					{ID: "branch-3", CellID: "cell-1", Region: "us-east-1"},
				}, nil)

				for _, branchID := range []string{"branch-1", "branch-2", "branch-3"} {
					bid := branchID
					cellClient := cellsmock.NewCellClient(t)
					mockCells.EXPECT().GetCellConnection(mock.Anything, orgID, "cell-1").Return(cellClient, nil).Once()
					cellClient.EXPECT().DeletePostgresCluster(mock.Anything, &clustersv1.DeletePostgresClusterRequest{Id: bid}).Return(&clustersv1.DeletePostgresClusterResponse{}, nil)
					cellClient.EXPECT().DeleteBranchIPFiltering(mock.Anything, &clustersv1.DeleteBranchIPFilteringRequest{BranchId: bid}).Return(&clustersv1.DeleteBranchIPFilteringResponse{}, nil)
					cellClient.EXPECT().Close().Return(nil)

					projectID := "proj-1"
					if bid == "branch-3" {
						projectID = "proj-2"
					}
					mockStore.EXPECT().GetPrimaryCell(mock.Anything, orgID, "us-east-1").Return(&store.Cell{ID: "cell-1"}, nil).Once()
					mockStore.EXPECT().DeleteBranch(mock.Anything, orgID, projectID, bid, mock.AnythingOfType("func(*store.Branch) error")).
						Run(func(_ context.Context, _, _, _ string, fn func(*store.Branch) error) {
							_ = fn(&store.Branch{ID: bid, CellID: "cell-1", Region: "us-east-1"})
						}).Return(nil)
				}
				mockStore.EXPECT().DeleteProject(mock.Anything, orgID, "proj-1").Return(nil)
				mockStore.EXPECT().DeleteProject(mock.Anything, orgID, "proj-2").Return(nil)
			},
			wantProjectsDel: 2,
			wantBranchesDel: 3,
		},
		"list projects error": {
			setupMock: func(mockStore *mocks.ProjectsStore, mockCells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, orgID).Return(nil, errTest)
			},
			wantErr: true,
		},
		"list branches error records error and continues": {
			setupMock: func(mockStore *mocks.ProjectsStore, mockCells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, orgID).Return([]store.Project{
					{ID: "proj-1"},
					{ID: "proj-2"},
				}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, orgID, "proj-1").Return(nil, errTest)
				mockStore.EXPECT().ListBranches(mock.Anything, orgID, "proj-2").Return(nil, nil)
				mockStore.EXPECT().DeleteProject(mock.Anything, orgID, "proj-2").Return(nil)
			},
			// proj-1 records an error, but proj-2 should still be deleted successfully.
			wantProjectsDel: 1,
			wantErrors:      []string{"list branches for project proj-1: test error"},
		},
		"delete branch error records error and continues": {
			setupMock: func(mockStore *mocks.ProjectsStore, mockCells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, orgID).Return([]store.Project{{ID: "proj-1"}}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, orgID, "proj-1").Return([]store.Branch{
					{ID: "branch-1", CellID: "cell-1", Region: "us-east-1"},
					{ID: "branch-2", CellID: "cell-1", Region: "us-east-1"},
				}, nil)

				for _, branchID := range []string{"branch-1", "branch-2"} {
					cellClient := cellsmock.NewCellClient(t)
					mockCells.EXPECT().GetCellConnection(mock.Anything, orgID, "cell-1").Return(cellClient, nil).Once()
					cellClient.EXPECT().DeletePostgresCluster(mock.Anything, &clustersv1.DeletePostgresClusterRequest{Id: branchID}).Return(&clustersv1.DeletePostgresClusterResponse{}, nil)
					cellClient.EXPECT().DeleteBranchIPFiltering(mock.Anything, &clustersv1.DeleteBranchIPFilteringRequest{BranchId: branchID}).Return(&clustersv1.DeleteBranchIPFilteringResponse{}, nil)
					cellClient.EXPECT().Close().Return(nil)
					mockStore.EXPECT().GetPrimaryCell(mock.Anything, orgID, "us-east-1").Return(&store.Cell{ID: "cell-1"}, nil).Once()
				}

				mockStore.EXPECT().DeleteBranch(mock.Anything, orgID, "proj-1", "branch-1", mock.AnythingOfType("func(*store.Branch) error")).
					Run(func(_ context.Context, _, _, _ string, fn func(*store.Branch) error) {
						_ = fn(&store.Branch{ID: "branch-1", CellID: "cell-1", Region: "us-east-1"})
					}).Return(errTest)
				mockStore.EXPECT().DeleteBranch(mock.Anything, orgID, "proj-1", "branch-2", mock.AnythingOfType("func(*store.Branch) error")).
					Run(func(_ context.Context, _, _, _ string, fn func(*store.Branch) error) {
						_ = fn(&store.Branch{ID: "branch-2", CellID: "cell-1", Region: "us-east-1"})
					}).Return(nil)
			},
			wantBranchesDel: 1,
			wantErrors:      []string{"delete branch branch-1: test error"},
		},
		"delete project error records error": {
			setupMock: func(mockStore *mocks.ProjectsStore, mockCells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, orgID).Return([]store.Project{{ID: "proj-1"}}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, orgID, "proj-1").Return(nil, nil)
				mockStore.EXPECT().DeleteProject(mock.Anything, orgID, "proj-1").Return(errTest)
			},
			wantErrors: []string{"delete project proj-1: test error"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			mockStore := mocks.NewProjectsStore(t)
			mockCells := cellsmock.NewCells(t)
			tt.setupMock(mockStore, mockCells)
			if tt.wantErrors == nil && !tt.wantErr {
				mockStore.EXPECT().DeleteOrganizationStatus(mock.Anything, orgID).Return(nil).Once()
			}

			service := NewProjectsService(mockStore, mockCells)
			resp, err := service.DeleteProjectsInOrg(ctx, &projectsv1.DeleteProjectsInOrgRequest{
				OrganizationId: orgID,
			})

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, orgID, resp.OrganizationId)
			require.Equal(t, tt.wantProjectsDel, resp.ProjectsDeleted)
			require.Equal(t, tt.wantBranchesDel, resp.BranchesDeleted)
			if tt.wantErrors != nil {
				require.Equal(t, tt.wantErrors, resp.Errors)
			} else {
				require.Empty(t, resp.Errors)
			}
		})
	}
}

func TestHasActiveProjects(t *testing.T) {
	ctx := context.Background()
	orgID := apitest.TestOrganization

	tests := map[string]struct {
		setupMock func(*mocks.ProjectsStore)
		want      bool
		wantErr   bool
	}{
		"has active projects": {
			setupMock: func(mockStore *mocks.ProjectsStore) {
				mockStore.EXPECT().ListProjects(mock.Anything, orgID).Return([]store.Project{{ID: "proj-1"}}, nil)
			},
			want: true,
		},
		"no active projects": {
			setupMock: func(mockStore *mocks.ProjectsStore) {
				mockStore.EXPECT().ListProjects(mock.Anything, orgID).Return(nil, nil)
			},
			want: false,
		},
		"list projects error": {
			setupMock: func(mockStore *mocks.ProjectsStore) {
				mockStore.EXPECT().ListProjects(mock.Anything, orgID).Return(nil, errTest)
			},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			mockStore := mocks.NewProjectsStore(t)
			tt.setupMock(mockStore)

			service := NewProjectsService(mockStore, nil)
			got, err := service.HasActiveProjects(ctx, &projectsv1.HasActiveProjectsRequest{
				OrganizationId: orgID,
			})

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got.HasActiveProjects)
		})
	}
}

var errTest = fmt.Errorf("test error")

// fakeNudger records whether the handler woke the orgstatus worker.
type fakeNudger struct{ nudges int }

func (f *fakeNudger) Nudge() { f.nudges++ }

func TestUpdateOrganizationStatus(t *testing.T) {
	request := &projectsv1.UpdateOrganizationStatusRequest{
		OrganizationId: apitest.TestOrganization,
		Disabled:       true,
	}

	t.Run("records the desired state and wakes the worker", func(t *testing.T) {
		mockStore := mocks.NewProjectsStore(t)
		mockCells := cellsmock.NewCells(t)
		mockStore.EXPECT().UpsertOrganizationStatus(mock.Anything, apitest.TestOrganization, true).
			Return(&store.OrganizationStatus{OrganizationID: apitest.TestOrganization, Disabled: true, Version: 1}, nil).Once()

		nudger := &fakeNudger{}
		service := NewProjectsService(mockStore, mockCells)
		service.SetNudger(nudger)

		got, err := service.UpdateOrganizationStatus(context.Background(), request)

		require.NoError(t, err)
		require.Equal(t, apitest.TestOrganization, got.OrganizationId)
		require.Equal(t, 1, nudger.nudges)
	})

	t.Run("a store failure is returned", func(t *testing.T) {
		mockStore := mocks.NewProjectsStore(t)
		mockCells := cellsmock.NewCells(t)
		mockStore.EXPECT().UpsertOrganizationStatus(mock.Anything, apitest.TestOrganization, true).
			Return(nil, errors.New("db down")).Once()

		_, err := NewProjectsService(mockStore, mockCells).UpdateOrganizationStatus(context.Background(), request)

		require.ErrorContains(t, err, apitest.TestOrganization)
		require.ErrorContains(t, err, "db down")
	})

	t.Run("no nudger configured is not fatal", func(t *testing.T) {
		mockStore := mocks.NewProjectsStore(t)
		mockCells := cellsmock.NewCells(t)
		mockStore.EXPECT().UpsertOrganizationStatus(mock.Anything, apitest.TestOrganization, true).
			Return(&store.OrganizationStatus{OrganizationID: apitest.TestOrganization}, nil).Once()

		_, err := NewProjectsService(mockStore, mockCells).UpdateOrganizationStatus(context.Background(), request)

		require.NoError(t, err)
	})
}
