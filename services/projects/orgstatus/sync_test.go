package orgstatus

import (
	"context"
	"testing"

	clustersv1 "xata/gen/proto/clusters/v1"
	"xata/internal/apitest"
	"xata/services/projects/cells/cellsmock"
	"xata/services/projects/store"
	"xata/services/projects/store/mocks"

	projectsv1 "xata/gen/proto/projects/v1"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSync(t *testing.T) {
	ctx := context.Background()
	tests := map[string]struct {
		setupMock   func(*mocks.ProjectsStore, *cellsmock.Cells)
		authRequest *projectsv1.UpdateOrganizationStatusRequest
	}{
		"disable org with S2Z configured branches": {
			setupMock: func(mockStore *mocks.ProjectsStore, cells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).Return([]store.Project{
					{ID: "proj-1"},
					{ID: "proj-2"},
				}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, apitest.TestOrganization, "proj-1").Return([]store.Branch{
					{ID: "branch-1", CellID: "cell-1"},
				}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, apitest.TestOrganization, "proj-2").Return([]store.Branch{
					{ID: "branch-2", CellID: "cell-1"},
				}, nil)
				cellClient := cellsmock.NewCellClient(t)
				cells.EXPECT().GetCellConnection(mock.Anything, apitest.TestOrganization, "cell-1").Return(cellClient, nil)
				cellClient.EXPECT().Close().Return(nil)
				cellClient.EXPECT().DescribePostgresCluster(mock.Anything, &clustersv1.DescribePostgresClusterRequest{
					Id: "branch-1",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Id: "branch-1",
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						Hibernate: false,
						ScaleToZero: &clustersv1.ScaleToZero{
							Enabled:                 true,
							InactivityPeriodMinutes: 30,
						},
					},
				}, nil)
				cellClient.EXPECT().DescribePostgresCluster(mock.Anything, &clustersv1.DescribePostgresClusterRequest{
					Id: "branch-2",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Id: "branch-2",
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						Hibernate: false,
						ScaleToZero: &clustersv1.ScaleToZero{
							Enabled:                 true,
							InactivityPeriodMinutes: 30,
						},
					},
				}, nil)
				cellClient.EXPECT().UpdatePostgresCluster(mock.Anything, &clustersv1.UpdatePostgresClusterRequest{
					Id: "branch-1",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						Hibernate: new(true),
						ScaleToZero: &clustersv1.ScaleToZero{
							Enabled:                 false,
							InactivityPeriodMinutes: 30,
						},
					},
				}).Return(&clustersv1.UpdatePostgresClusterResponse{}, nil)
				cellClient.EXPECT().UpdatePostgresCluster(mock.Anything, &clustersv1.UpdatePostgresClusterRequest{
					Id: "branch-2",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						Hibernate: new(true),
						ScaleToZero: &clustersv1.ScaleToZero{
							Enabled:                 false,
							InactivityPeriodMinutes: 30,
						},
					},
				}).Return(&clustersv1.UpdatePostgresClusterResponse{}, nil)
			},
			authRequest: &projectsv1.UpdateOrganizationStatusRequest{
				OrganizationId: apitest.TestOrganization,
				Disabled:       true,
			},
		},
		"disable already disabled branch is a no-op": {
			setupMock: func(mockStore *mocks.ProjectsStore, cells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).Return([]store.Project{{ID: "proj-1"}}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, apitest.TestOrganization, "proj-1").Return([]store.Branch{
					{ID: "branch-1", CellID: "cell-1"},
				}, nil)
				cellClient := cellsmock.NewCellClient(t)
				cells.EXPECT().GetCellConnection(mock.Anything, apitest.TestOrganization, "cell-1").Return(cellClient, nil)
				cellClient.EXPECT().Close().Return(nil)
				cellClient.EXPECT().DescribePostgresCluster(mock.Anything, &clustersv1.DescribePostgresClusterRequest{
					Id: "branch-1",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Id: "branch-1",
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						Hibernate: true,
						ScaleToZero: &clustersv1.ScaleToZero{
							Enabled:                 false,
							InactivityPeriodMinutes: 30,
						},
					},
				}, nil)
			},
			authRequest: &projectsv1.UpdateOrganizationStatusRequest{
				OrganizationId: apitest.TestOrganization,
				Disabled:       true,
			},
		},
		"disable hibernated branch with S2Z enabled disables S2Z": {
			setupMock: func(mockStore *mocks.ProjectsStore, cells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).Return([]store.Project{{ID: "proj-1"}}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, apitest.TestOrganization, "proj-1").Return([]store.Branch{
					{ID: "branch-1", CellID: "cell-1"},
				}, nil)
				cellClient := cellsmock.NewCellClient(t)
				cells.EXPECT().GetCellConnection(mock.Anything, apitest.TestOrganization, "cell-1").Return(cellClient, nil)
				cellClient.EXPECT().Close().Return(nil)
				cellClient.EXPECT().DescribePostgresCluster(mock.Anything, &clustersv1.DescribePostgresClusterRequest{
					Id: "branch-1",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Id: "branch-1",
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						Hibernate: false,
						ScaleToZero: &clustersv1.ScaleToZero{
							Enabled:                 true,
							InactivityPeriodMinutes: 30,
						},
					},
				}, nil)
				cellClient.EXPECT().UpdatePostgresCluster(mock.Anything, &clustersv1.UpdatePostgresClusterRequest{
					Id: "branch-1",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						ScaleToZero: &clustersv1.ScaleToZero{
							Enabled:                 false,
							InactivityPeriodMinutes: 30,
						},
					},
				}).Return(&clustersv1.UpdatePostgresClusterResponse{}, nil)
			},
			authRequest: &projectsv1.UpdateOrganizationStatusRequest{
				OrganizationId: apitest.TestOrganization,
				Disabled:       true,
			},
		},
		"disable org with branch without S2Z config skips S2Z update": {
			setupMock: func(mockStore *mocks.ProjectsStore, cells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).Return([]store.Project{{ID: "proj-1"}}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, apitest.TestOrganization, "proj-1").Return([]store.Branch{
					{ID: "branch-1", CellID: "cell-1"},
				}, nil)
				cellClient := cellsmock.NewCellClient(t)
				cells.EXPECT().GetCellConnection(mock.Anything, apitest.TestOrganization, "cell-1").Return(cellClient, nil)
				cellClient.EXPECT().Close().Return(nil)
				cellClient.EXPECT().DescribePostgresCluster(mock.Anything, &clustersv1.DescribePostgresClusterRequest{
					Id: "branch-1",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Id: "branch-1",
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HEALTHY,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						Hibernate: false,
					},
				}, nil)
				cellClient.EXPECT().UpdatePostgresCluster(mock.Anything, &clustersv1.UpdatePostgresClusterRequest{
					Id: "branch-1",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						Hibernate: new(true),
					},
				}).Return(&clustersv1.UpdatePostgresClusterResponse{}, nil)
			},
			authRequest: &projectsv1.UpdateOrganizationStatusRequest{
				OrganizationId: apitest.TestOrganization,
				Disabled:       true,
			},
		},
		"re-enable org restores S2Z on branch that had it configured": {
			setupMock: func(mockStore *mocks.ProjectsStore, cells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).Return([]store.Project{{ID: "proj-1"}}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, apitest.TestOrganization, "proj-1").Return([]store.Branch{
					{ID: "branch-1", CellID: "cell-1"},
				}, nil)
				cellClient := cellsmock.NewCellClient(t)
				cells.EXPECT().GetCellConnection(mock.Anything, apitest.TestOrganization, "cell-1").Return(cellClient, nil)
				cellClient.EXPECT().Close().Return(nil)
				cellClient.EXPECT().DescribePostgresCluster(mock.Anything, &clustersv1.DescribePostgresClusterRequest{
					Id: "branch-1",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Id: "branch-1",
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						Hibernate: true,
						ScaleToZero: &clustersv1.ScaleToZero{
							Enabled:                 false,
							InactivityPeriodMinutes: 30,
						},
					},
				}, nil)
				cellClient.EXPECT().UpdatePostgresCluster(mock.Anything, &clustersv1.UpdatePostgresClusterRequest{
					Id: "branch-1",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						Hibernate: new(false),
						ScaleToZero: &clustersv1.ScaleToZero{
							Enabled:                 true,
							InactivityPeriodMinutes: 30,
						},
					},
				}).Return(&clustersv1.UpdatePostgresClusterResponse{}, nil)
			},
			authRequest: &projectsv1.UpdateOrganizationStatusRequest{
				OrganizationId: apitest.TestOrganization,
				Disabled:       false,
			},
		},
		"re-enable org skips S2Z on branch that never had it": {
			setupMock: func(mockStore *mocks.ProjectsStore, cells *cellsmock.Cells) {
				mockStore.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).Return([]store.Project{{ID: "proj-1"}}, nil)
				mockStore.EXPECT().ListBranches(mock.Anything, apitest.TestOrganization, "proj-1").Return([]store.Branch{
					{ID: "branch-1", CellID: "cell-1"},
				}, nil)
				cellClient := cellsmock.NewCellClient(t)
				cells.EXPECT().GetCellConnection(mock.Anything, apitest.TestOrganization, "cell-1").Return(cellClient, nil)
				cellClient.EXPECT().Close().Return(nil)
				cellClient.EXPECT().DescribePostgresCluster(mock.Anything, &clustersv1.DescribePostgresClusterRequest{
					Id: "branch-1",
				}).Return(&clustersv1.DescribePostgresClusterResponse{
					Id: "branch-1",
					Status: &clustersv1.ClusterStatus{
						StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED,
					},
					Configuration: &clustersv1.ClusterConfiguration{
						Hibernate: true,
					},
				}, nil)
				cellClient.EXPECT().UpdatePostgresCluster(mock.Anything, &clustersv1.UpdatePostgresClusterRequest{
					Id: "branch-1",
					UpdateConfiguration: &clustersv1.UpdateClusterConfiguration{
						Hibernate: new(false),
					},
				}).Return(&clustersv1.UpdatePostgresClusterResponse{}, nil)
			},
			authRequest: &projectsv1.UpdateOrganizationStatusRequest{
				OrganizationId: apitest.TestOrganization,
				Disabled:       false,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			mockStore := mocks.NewProjectsStore(t)
			mockCells := cellsmock.NewCells(t)
			tt.setupMock(mockStore, mockCells)

			err := Sync(ctx, mockStore, mockCells, tt.authRequest.OrganizationId, tt.authRequest.Disabled)

			require.NoError(t, err)
		})
	}
}

func TestSyncStopsWhenContextEnds(t *testing.T) {
	request := &projectsv1.UpdateOrganizationStatusRequest{
		OrganizationId: apitest.TestOrganization,
		Disabled:       true,
	}

	t.Run("stops before the first branch", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		mockStore := mocks.NewProjectsStore(t)
		mockCells := cellsmock.NewCells(t)
		mockStore.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).
			Return([]store.Project{{ID: "proj-1"}}, nil)
		mockStore.EXPECT().ListBranches(mock.Anything, apitest.TestOrganization, "proj-1").
			Return([]store.Branch{
				{ID: "branch-1", CellID: "cell-1"},
				{ID: "branch-2", CellID: "cell-1"},
			}, nil)

		// No cell connection is opened and no cluster is described, so the
		// mocks contain no expectations for either.
		err := Sync(ctx, mockStore, mockCells, request.OrganizationId, request.Disabled)

		require.Error(t, err)
		require.Equal(t, codes.Canceled, status.Code(err))
		require.Contains(t, err.Error(), apitest.TestOrganization)
		require.Contains(t, err.Error(), "aborted after 0 branches (0 updated)")
	})

	t.Run("stops at the branch after the context ends", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mockStore := mocks.NewProjectsStore(t)
		mockCells := cellsmock.NewCells(t)
		mockStore.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).
			Return([]store.Project{{ID: "proj-1"}}, nil)
		mockStore.EXPECT().ListBranches(mock.Anything, apitest.TestOrganization, "proj-1").
			Return([]store.Branch{
				{ID: "branch-1", CellID: "cell-1"},
				{ID: "branch-2", CellID: "cell-1"},
				{ID: "branch-3", CellID: "cell-1"},
			}, nil)

		cellClient := cellsmock.NewCellClient(t)
		mockCells.EXPECT().GetCellConnection(mock.Anything, apitest.TestOrganization, "cell-1").
			Return(cellClient, nil)
		cellClient.EXPECT().Close().Return(nil)

		// branch-1 is already hibernated and so needs no update. Ending the
		// context inside the call stands in for the caller giving up part way
		// through, which is what an Orb webhook timeout does in production.
		cellClient.EXPECT().DescribePostgresCluster(mock.Anything, &clustersv1.DescribePostgresClusterRequest{
			Id: "branch-1",
		}).Run(func(_ context.Context, _ *clustersv1.DescribePostgresClusterRequest, _ ...grpc.CallOption) {
			cancel()
		}).Return(&clustersv1.DescribePostgresClusterResponse{
			Id:     "branch-1",
			Status: &clustersv1.ClusterStatus{StatusType: clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED},
			Configuration: &clustersv1.ClusterConfiguration{
				Hibernate:   true,
				ScaleToZero: &clustersv1.ScaleToZero{Enabled: false, InactivityPeriodMinutes: 30},
			},
		}, nil)

		err := Sync(ctx, mockStore, mockCells, request.OrganizationId, request.Disabled)

		require.Error(t, err)
		require.Equal(t, codes.Canceled, status.Code(err))
		require.Contains(t, err.Error(), "aborted after 1 branches (0 updated)")

		// branch-2 and branch-3 are never described. Before this change each
		// of them produced its own error log line.
		cellClient.AssertNumberOfCalls(t, "DescribePostgresCluster", 1)
	})
}
