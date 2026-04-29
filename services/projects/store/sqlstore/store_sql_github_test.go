package sqlstore

import (
	"context"
	"testing"

	"xata/services/projects/store"

	"github.com/stretchr/testify/require"
)

func TestSQLStoreGithubInstallations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sqlStore := setupSQLStore(ctx, t, maxDepth, maxChildren)

	t.Run("list github installations", func(t *testing.T) {
		orgWithInstalls := "gh-list-org"
		_, err := sqlStore.CreateGithubInstallation(ctx, orgWithInstalls, 12345)
		require.NoError(t, err)

		tests := map[string]struct {
			organization string
			wantCount    int
		}{
			"empty org returns no installations": {
				organization: "gh-empty-org",
				wantCount:    0,
			},
			"org with installations returns rows": {
				organization: orgWithInstalls,
				wantCount:    1,
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := sqlStore.ListGithubInstallations(ctx, tt.organization)
				require.NoError(t, err)
				require.Len(t, got, tt.wantCount)
			})
		}
	})

	t.Run("create github installation", func(t *testing.T) {
		existingOrg := "gh-create-existing-org"
		var existingInstallationID int64 = 12345
		_, err := sqlStore.CreateGithubInstallation(ctx, existingOrg, existingInstallationID)
		require.NoError(t, err)

		tests := map[string]struct {
			organization   string
			installationID int64
			wantError      bool
			checkError     func(t *testing.T, err error)
		}{
			"creates installation": {
				organization:   "gh-create-org",
				installationID: 99999,
			},
			"duplicate installation in same org returns error": {
				organization:   existingOrg,
				installationID: existingInstallationID,
				wantError:      true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrGithubInstallationAlreadyExists
					require.ErrorAs(t, err, &target)
				},
			},
			"same installation id in different org succeeds": {
				organization:   "gh-create-other-org",
				installationID: existingInstallationID,
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := sqlStore.CreateGithubInstallation(ctx, tt.organization, tt.installationID)
				if tt.wantError {
					require.Error(t, err)
					tt.checkError(t, err)
					return
				}
				require.NoError(t, err)
				require.NotEmpty(t, got.ID)
				require.Equal(t, tt.organization, got.Organization)
				require.Equal(t, tt.installationID, got.InstallationID)
			})
		}
	})

	t.Run("update github installation", func(t *testing.T) {
		updateOrg := "gh-update-org"
		inst, err := sqlStore.CreateGithubInstallation(ctx, updateOrg, 12345)
		require.NoError(t, err)

		conflictOrg := "gh-update-conflict-org"
		conflictInst, err := sqlStore.CreateGithubInstallation(ctx, conflictOrg, 11111)
		require.NoError(t, err)
		_, err = sqlStore.CreateGithubInstallation(ctx, conflictOrg, 22222)
		require.NoError(t, err)

		tests := map[string]struct {
			organization      string
			installationID    string
			newInstallationID int64
			wantError         bool
			checkError        func(t *testing.T, err error)
		}{
			"updates installation id": {
				organization:      updateOrg,
				installationID:    inst.ID,
				newInstallationID: 99999,
			},
			"unknown installation returns not found": {
				organization:      updateOrg,
				installationID:    "unknown-id",
				newInstallationID: 99999,
				wantError:         true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrGithubInstallationNotFound
					require.ErrorAs(t, err, &target)
				},
			},
			"duplicate installation id in same org returns error": {
				organization:      conflictOrg,
				installationID:    conflictInst.ID,
				newInstallationID: 22222,
				wantError:         true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrGithubInstallationAlreadyExists
					require.ErrorAs(t, err, &target)
				},
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := sqlStore.UpdateGithubInstallation(ctx, tt.organization, tt.installationID, tt.newInstallationID)
				if tt.wantError {
					require.Error(t, err)
					tt.checkError(t, err)
					return
				}
				require.NoError(t, err)
				require.Equal(t, tt.newInstallationID, got.InstallationID)
				require.True(t, got.UpdatedAt.After(inst.UpdatedAt))
			})
		}
	})

	t.Run("delete github installation", func(t *testing.T) {
		createRegionAndCell(t, sqlStore, "gh-del-inst-region", "gh-del-inst-cell")

		createProjectAndBranch := func(t *testing.T, org, projectName, branchName string) (projectID, branchID string) {
			t.Helper()
			project, err := sqlStore.CreateProject(ctx, org, createProjectConfig(projectName, nil))
			require.NoError(t, err)
			branch, err := sqlStore.CreateBranch(ctx, org, project.ID, "gh-del-inst-cell", createBranchConfig(branchName, nil, nil), noopProvisionFunc)
			require.NoError(t, err)
			return project.ID, branch.ID
		}

		t.Run("deletes installation and leaves repo mappings intact", func(t *testing.T) {
			org := "gh-del-inst-org"
			_, err := sqlStore.CreateGithubInstallation(ctx, org, 60001)
			require.NoError(t, err)

			projectID, branchID := createProjectAndBranch(t, org, "gh-del-inst-project", "main")
			_, err = sqlStore.CreateGithubRepoMapping(ctx, org, projectID, 60001, branchID)
			require.NoError(t, err)

			err = sqlStore.DeleteGithubInstallation(ctx, 60001)
			require.NoError(t, err)

			installs, err := sqlStore.ListGithubInstallations(ctx, org)
			require.NoError(t, err)
			require.Len(t, installs, 0)

			got, err := sqlStore.GetGithubRepoMappingByRepoID(ctx, 60001)
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, int64(60001), got.GithubRepositoryID)
		})

		t.Run("deletes installation with no repo mappings", func(t *testing.T) {
			org := "gh-del-inst-nomapping-org"
			_, err := sqlStore.CreateGithubInstallation(ctx, org, 60002)
			require.NoError(t, err)

			err = sqlStore.DeleteGithubInstallation(ctx, 60002)
			require.NoError(t, err)

			installs, err := sqlStore.ListGithubInstallations(ctx, org)
			require.NoError(t, err)
			require.Len(t, installs, 0)
		})

		t.Run("nonexistent installation succeeds silently", func(t *testing.T) {
			err := sqlStore.DeleteGithubInstallation(ctx, 99999)
			require.NoError(t, err)
		})

		t.Run("leaves unrelated installation untouched", func(t *testing.T) {
			orgA := "gh-del-inst-orgA"
			_, err := sqlStore.CreateGithubInstallation(ctx, orgA, 60003)
			require.NoError(t, err)
			projectA, branchA := createProjectAndBranch(t, orgA, "gh-del-inst-projA", "main")
			_, err = sqlStore.CreateGithubRepoMapping(ctx, orgA, projectA, 70001, branchA)
			require.NoError(t, err)

			orgB := "gh-del-inst-orgB"
			_, err = sqlStore.CreateGithubInstallation(ctx, orgB, 60004)
			require.NoError(t, err)
			projectB, branchB := createProjectAndBranch(t, orgB, "gh-del-inst-projB", "main")
			_, err = sqlStore.CreateGithubRepoMapping(ctx, orgB, projectB, 70002, branchB)
			require.NoError(t, err)

			err = sqlStore.DeleteGithubInstallation(ctx, 60003)
			require.NoError(t, err)

			installs, err := sqlStore.ListGithubInstallations(ctx, orgB)
			require.NoError(t, err)
			require.Len(t, installs, 1)

			got, err := sqlStore.GetGithubRepoMappingByRepoID(ctx, 70002)
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, int64(70002), got.GithubRepositoryID)
		})
	})
}

func TestSQLStoreGithubRepoMappings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sqlStore := setupSQLStore(ctx, t, maxDepth, maxChildren)
	createRegionAndCell(t, sqlStore, "region", "cell")

	createProjectAndBranch := func(t *testing.T, org, projectName, branchName string) (projectID, branchID string) {
		t.Helper()
		project, err := sqlStore.CreateProject(ctx, org, createProjectConfig(projectName, nil))
		require.NoError(t, err)
		branch, err := sqlStore.CreateBranch(ctx, org, project.ID, "cell", createBranchConfig(branchName, nil, nil), noopProvisionFunc)
		require.NoError(t, err)
		return project.ID, branch.ID
	}

	createMapping := func(t *testing.T, org, projectID string, repoID int64, branchID string) string {
		t.Helper()
		mapping, err := sqlStore.CreateGithubRepoMapping(ctx, org, projectID, repoID, branchID)
		require.NoError(t, err)
		require.NotEmpty(t, mapping.ID)
		return mapping.ID
	}

	t.Run("get github repo mapping by project", func(t *testing.T) {
		org := "gh-repo-get-org"

		noMapProjectID, _ := createProjectAndBranch(t, org, "gh-project-no-map", "main")

		mappedProjectID, mappedBranchID := createProjectAndBranch(t, org, "gh-project-mapped", "main")
		expectedMappingID := createMapping(t, org, mappedProjectID, 1042, mappedBranchID)

		otherOrg := "gh-repo-get-other-org"
		otherProjectID, otherBranchID := createProjectAndBranch(t, otherOrg, "gh-project-other-org", "main")
		createMapping(t, otherOrg, otherProjectID, 200, otherBranchID)

		terminatedProjectID, terminatedBranchID := createProjectAndBranch(t, org, "gh-project-terminated", "main")
		terminatedMappingID := createMapping(t, org, terminatedProjectID, 1099, terminatedBranchID)
		err := sqlStore.DeleteBranch(ctx, org, terminatedProjectID, terminatedBranchID, func(*store.Branch) error { return nil })
		require.NoError(t, err)

		tests := map[string]struct {
			org        string
			projectID  string
			wantError  bool
			checkError func(t *testing.T, err error)
			wantID     string
		}{
			"returns not found when project has no mapping": {
				org:       org,
				projectID: noMapProjectID,
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrGithubRepoMappingNotFound
					require.ErrorAs(t, err, &target)
				},
			},
			"returns mapping when project has mapping": {
				org:       org,
				projectID: mappedProjectID,
				wantID:    expectedMappingID,
			},
			"returns project not found for project in another org": {
				org:       org,
				projectID: otherProjectID,
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrProjectNotFound
					require.ErrorAs(t, err, &target)
				},
			},
			"returns mapping even when root branch is terminated": {
				org:       org,
				projectID: terminatedProjectID,
				wantID:    terminatedMappingID,
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := sqlStore.GetGithubRepoMappingByProject(ctx, tt.org, tt.projectID)
				if tt.wantError {
					require.Error(t, err)
					tt.checkError(t, err)
					return
				}
				require.NoError(t, err)
				require.NotNil(t, got)
				require.Equal(t, tt.wantID, got.ID)
			})
		}
	})

	t.Run("create github repo mapping", func(t *testing.T) {
		org := "gh-repo-create-org"

		newProjectID, newBranchID := createProjectAndBranch(t, org, "gh-project-create", "main")

		dupProjectID, dupBranchID := createProjectAndBranch(t, org, "gh-project-dup", "main")
		createMapping(t, org, dupProjectID, 2099, dupBranchID)

		repoIDTakenProjectID, repoIDTakenBranchID := createProjectAndBranch(t, org, "gh-project-repoid-taken", "main")
		createMapping(t, org, repoIDTakenProjectID, 2050, repoIDTakenBranchID)
		reusedRepoProjectID, reusedRepoBranchID := createProjectAndBranch(t, org, "gh-project-reuse-repo", "main")

		_, otherBranchID := createProjectAndBranch(t, org, "gh-project-other", "main")

		tests := map[string]struct {
			org        string
			projectID  string
			repoID     int64
			branchID   string
			wantError  bool
			checkError func(t *testing.T, err error)
		}{
			"creates mapping": {
				org:       org,
				projectID: newProjectID,
				repoID:    2042,
				branchID:  newBranchID,
			},
			"duplicate mapping for same project returns error": {
				org:       org,
				projectID: dupProjectID,
				repoID:    100,
				branchID:  dupBranchID,
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrGithubRepoMappingAlreadyExists
					require.ErrorAs(t, err, &target)
				},
			},
			"nonexistent project returns error": {
				org:       org,
				projectID: "nonexistent-project",
				repoID:    2043,
				branchID:  otherBranchID,
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrProjectNotFound
					require.ErrorAs(t, err, &target)
				},
			},
			"repo id already mapped to another project returns error": {
				org:       org,
				projectID: reusedRepoProjectID,
				repoID:    2050,
				branchID:  reusedRepoBranchID,
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrGithubRepositoryAlreadyMapped
					require.ErrorAs(t, err, &target)
				},
			},
			"branch from another project returns error": {
				org:       org,
				projectID: newProjectID,
				repoID:    2044,
				branchID:  otherBranchID,
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrBranchNotFound
					require.ErrorAs(t, err, &target)
				},
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := sqlStore.CreateGithubRepoMapping(ctx, tt.org, tt.projectID, tt.repoID, tt.branchID)
				if tt.wantError {
					require.Error(t, err)
					tt.checkError(t, err)
					return
				}
				require.NoError(t, err)
				require.NotEmpty(t, got.ID)
				require.Equal(t, tt.repoID, got.GithubRepositoryID)
				require.Equal(t, tt.projectID, got.Project)
				require.Equal(t, tt.branchID, got.RootBranchID)
			})
		}
	})

	t.Run("update github repo mapping", func(t *testing.T) {
		org := "gh-repo-update-org"

		project, err := sqlStore.CreateProject(ctx, org, createProjectConfig("gh-project-update", nil))
		require.NoError(t, err)
		mainBranch, err := sqlStore.CreateBranch(ctx, org, project.ID, "cell", createBranchConfig("main", nil, nil), noopProvisionFunc)
		require.NoError(t, err)
		devBranch, err := sqlStore.CreateBranch(ctx, org, project.ID, "cell", createBranchConfig("dev", nil, nil), noopProvisionFunc)
		require.NoError(t, err)
		createMapping(t, org, project.ID, 3042, mainBranch.ID)

		conflictProject, err := sqlStore.CreateProject(ctx, org, createProjectConfig("gh-project-conflict-repo", nil))
		require.NoError(t, err)
		conflictBranch, err := sqlStore.CreateBranch(ctx, org, conflictProject.ID, "cell", createBranchConfig("main", nil, nil), noopProvisionFunc)
		require.NoError(t, err)
		createMapping(t, org, conflictProject.ID, 3300, conflictBranch.ID)

		noMapProjectID, noMapBranchID := createProjectAndBranch(t, org, "gh-project-no-mapping", "main")
		_, otherBranchID := createProjectAndBranch(t, org, "gh-project-other-update", "main")

		otherOrg := "gh-repo-update-other-org"
		otherOrgProjectID, otherOrgBranchID := createProjectAndBranch(t, otherOrg, "gh-project-other-org-update", "main")
		createMapping(t, otherOrg, otherOrgProjectID, 3500, otherOrgBranchID)

		tests := map[string]struct {
			projectID  string
			repoID     int64
			branchID   string
			wantError  bool
			checkError func(t *testing.T, err error)
		}{
			"updates mapping": {
				projectID: project.ID,
				repoID:    3100,
				branchID:  devBranch.ID,
			},
			"nonexistent project returns project not found": {
				projectID: "nonexistent-project",
				repoID:    3102,
				branchID:  devBranch.ID,
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrProjectNotFound
					require.ErrorAs(t, err, &target)
				},
			},
			"project in another org returns project not found": {
				projectID: otherOrgProjectID,
				repoID:    3103,
				branchID:  otherOrgBranchID,
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrProjectNotFound
					require.ErrorAs(t, err, &target)
				},
			},
			"project without mapping returns not found": {
				projectID: noMapProjectID,
				repoID:    3101,
				branchID:  noMapBranchID,
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrGithubRepoMappingNotFound
					require.ErrorAs(t, err, &target)
				},
			},
			"repo id already mapped to another project returns error": {
				projectID: project.ID,
				repoID:    3300,
				branchID:  devBranch.ID,
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrGithubRepositoryAlreadyMapped
					require.ErrorAs(t, err, &target)
				},
			},
			"branch from another project returns error": {
				projectID: project.ID,
				repoID:    3200,
				branchID:  otherBranchID,
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrBranchNotFound
					require.ErrorAs(t, err, &target)
				},
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := sqlStore.UpdateGithubRepoMapping(ctx, org, tt.projectID, tt.repoID, tt.branchID)
				if tt.wantError {
					require.Error(t, err)
					tt.checkError(t, err)
					return
				}
				require.NoError(t, err)
				require.Equal(t, tt.repoID, got.GithubRepositoryID)
				require.Equal(t, tt.branchID, got.RootBranchID)
			})
		}
	})

	t.Run("get github repo mapping by repo id", func(t *testing.T) {
		org := "gh-repo-byid-org"
		projectID, branchID := createProjectAndBranch(t, org, "gh-project-byid", "main")
		createMapping(t, org, projectID, 5042, branchID)

		otherOrg := "gh-repo-byid-other-org"
		otherProjectID, otherBranchID := createProjectAndBranch(t, otherOrg, "gh-project-byid-other", "main")
		createMapping(t, otherOrg, otherProjectID, 5043, otherBranchID)

		deletedOrg := "gh-repo-byid-deleted-org"
		deletedProjectID, deletedBranchID := createProjectAndBranch(t, deletedOrg, "gh-project-byid-deleted", "main")
		createMapping(t, deletedOrg, deletedProjectID, 5044, deletedBranchID)
		err := sqlStore.DeleteBranch(ctx, deletedOrg, deletedProjectID, deletedBranchID, func(*store.Branch) error { return nil })
		require.NoError(t, err)
		err = sqlStore.DeleteProject(ctx, deletedOrg, deletedProjectID)
		require.NoError(t, err)

		tests := map[string]struct {
			repoID       int64
			wantNotFound bool
			wantRepoID   int64
			wantOrg      string
		}{
			"returns mapping for known repo id": {
				repoID:     5042,
				wantRepoID: 5042,
				wantOrg:    org,
			},
			"returns not found for unknown repo id": {
				repoID:       99999,
				wantNotFound: true,
			},
			"returns not found when project is soft-deleted": {
				repoID:       5044,
				wantNotFound: true,
			},
			"returns correct mapping among multiple": {
				repoID:     5043,
				wantRepoID: 5043,
				wantOrg:    otherOrg,
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := sqlStore.GetGithubRepoMappingByRepoID(ctx, tt.repoID)
				if tt.wantNotFound {
					var target store.ErrGithubRepoMappingNotFound
					require.ErrorAs(t, err, &target)
					require.Equal(t, tt.repoID, target.RepoID)
					require.Nil(t, got)
					return
				}
				require.NoError(t, err)
				require.NotNil(t, got)
				require.Equal(t, tt.wantRepoID, got.GithubRepositoryID)
				require.Equal(t, tt.wantOrg, got.OrganizationID)
			})
		}
	})

	t.Run("delete github repo mapping", func(t *testing.T) {
		type deleteArgs struct {
			org       string
			projectID string
		}

		tests := map[string]struct {
			setup      func(t *testing.T) deleteArgs
			wantError  bool
			checkError func(t *testing.T, err error)
			skipVerify bool
		}{
			"deletes mapping": {
				setup: func(t *testing.T) deleteArgs {
					org := "gh-repo-delete-org"
					projectID, branchID := createProjectAndBranch(t, org, "gh-project-delete", "main")
					createMapping(t, org, projectID, 4042, branchID)
					return deleteArgs{org: org, projectID: projectID}
				},
			},
			"nonexistent project returns project not found": {
				setup: func(t *testing.T) deleteArgs {
					return deleteArgs{org: "gh-repo-delete-noproject-org", projectID: "nonexistent-project"}
				},
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrProjectNotFound
					require.ErrorAs(t, err, &target)
				},
			},
			"project in another org returns project not found": {
				setup: func(t *testing.T) deleteArgs {
					otherOrg := "gh-repo-delete-other-org"
					projectID, branchID := createProjectAndBranch(t, otherOrg, "gh-project-delete-other", "main")
					createMapping(t, otherOrg, projectID, 4044, branchID)
					return deleteArgs{org: "gh-repo-delete-wrong-org", projectID: projectID}
				},
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrProjectNotFound
					require.ErrorAs(t, err, &target)
				},
			},
			"deletes mapping from soft-deleted project": {
				setup: func(t *testing.T) deleteArgs {
					org := "gh-repo-delete-softdel-org"
					projectID, branchID := createProjectAndBranch(t, org, "gh-project-softdel", "main")
					createMapping(t, org, projectID, 4045, branchID)
					err := sqlStore.DeleteBranch(ctx, org, projectID, branchID, func(*store.Branch) error { return nil })
					require.NoError(t, err)
					err = sqlStore.DeleteProject(ctx, org, projectID)
					require.NoError(t, err)
					return deleteArgs{org: org, projectID: projectID}
				},
				skipVerify: true,
			},
			"deleting nonexistent mapping returns not found": {
				setup: func(t *testing.T) deleteArgs {
					org := "gh-repo-delete-again-org"
					projectID, branchID := createProjectAndBranch(t, org, "gh-project-delete-again", "main")
					createMapping(t, org, projectID, 4043, branchID)
					err := sqlStore.DeleteGithubRepoMapping(ctx, org, projectID)
					require.NoError(t, err)
					return deleteArgs{org: org, projectID: projectID}
				},
				wantError: true,
				checkError: func(t *testing.T, err error) {
					var target store.ErrGithubRepoMappingNotFound
					require.ErrorAs(t, err, &target)
				},
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				args := tt.setup(t)
				err := sqlStore.DeleteGithubRepoMapping(ctx, args.org, args.projectID)
				if tt.wantError {
					require.Error(t, err)
					tt.checkError(t, err)
					return
				}
				require.NoError(t, err)

				if !tt.skipVerify {
					got, err := sqlStore.GetGithubRepoMappingByProject(ctx, args.org, args.projectID)
					var target store.ErrGithubRepoMappingNotFound
					require.ErrorAs(t, err, &target)
					require.Nil(t, got)
				}
			})
		}
	})
}
