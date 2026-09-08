package sqlstore

import (
	"context"
	"testing"

	"xata/services/projects/store"

	"github.com/stretchr/testify/require"
)

func TestSQLStoreVercelResources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sqlStore := setupSQLStore(ctx, t, maxDepth)
	createRegionAndCell(t, sqlStore, "vr-region", "cell")

	const org = "vercel-org"

	newProject := func(t *testing.T, name string) string {
		t.Helper()
		project, err := sqlStore.CreateProject(ctx, org, createProjectConfig(name, nil))
		require.NoError(t, err)
		return project.ID
	}

	// newBranch creates a Xata branch (the FK target for a resource branch row).
	newBranch := func(t *testing.T, projectID, name string) string {
		t.Helper()
		branch, err := sqlStore.CreateBranch(ctx, org, projectID, "cell", createBranchConfig(name, nil, nil), noopProvisionFunc)
		require.NoError(t, err)
		return branch.ID
	}

	newResource := func(t *testing.T, resourceID, projectID string) {
		t.Helper()
		_, err := sqlStore.CreateVercelResource(ctx, org, &store.VercelResource{
			ResourceID:     resourceID,
			InstallationID: "icfg_1",
			ProductSlug:    "postgres",
			BillingPlanID:  "plan_free",
			XataProjectID:  projectID,
		})
		require.NoError(t, err)
	}

	t.Run("create and get round-trips all fields", func(t *testing.T) {
		projectID := newProject(t, "vr-roundtrip")
		resource := &store.VercelResource{
			ResourceID:     "res_roundtrip",
			InstallationID: "icfg_1",
			ProductSlug:    "postgres",
			BillingPlanID:  "plan_free",
			XataProjectID:  projectID,
			Name:           "My DB",
			Metadata:       map[string]any{"region": "us-east-1", "instanceType": "small"},
		}
		created, err := sqlStore.CreateVercelResource(ctx, org, resource)
		require.NoError(t, err)
		require.Equal(t, store.VercelResourceActive, created.Status)
		require.False(t, created.CreatedAt.IsZero())
		// The input is left untouched; the stored row comes back as a new value.
		require.Empty(t, resource.Status)
		require.True(t, resource.CreatedAt.IsZero())

		got, err := sqlStore.GetVercelResource(ctx, "icfg_1", "res_roundtrip")
		require.NoError(t, err)
		require.Equal(t, "icfg_1", got.InstallationID)
		require.Equal(t, "postgres", got.ProductSlug)
		require.Equal(t, "plan_free", got.BillingPlanID)
		require.Equal(t, projectID, got.XataProjectID)
		require.Equal(t, "My DB", got.Name)
		require.Equal(t, "us-east-1", got.Metadata["region"])
		require.Equal(t, store.VercelResourceActive, got.Status)
		require.Nil(t, got.DeletedAt)
	})

	t.Run("get missing returns not found", func(t *testing.T) {
		_, err := sqlStore.GetVercelResource(ctx, "icfg_1", "res_missing")
		require.ErrorAs(t, err, &store.ErrVercelResourceNotFound{})
	})

	t.Run("duplicate resource id conflicts", func(t *testing.T) {
		newResource(t, "res_dup", newProject(t, "vr-dup-id"))

		// A fresh project so the collision is on the id, not the project index.
		_, err := sqlStore.CreateVercelResource(ctx, org, &store.VercelResource{
			ResourceID: "res_dup", InstallationID: "icfg_1", ProductSlug: "postgres",
			BillingPlanID: "plan_free", XataProjectID: newProject(t, "vr-dup-id-2"),
		})
		require.ErrorAs(t, err, &store.ErrVercelResourceAlreadyExists{})

		// A taken id wins over a bad project: the id conflict is reported, not
		// project-not-found.
		_, err = sqlStore.CreateVercelResource(ctx, org, &store.VercelResource{
			ResourceID: "res_dup", InstallationID: "icfg_1", ProductSlug: "postgres",
			BillingPlanID: "plan_free", XataProjectID: "prj_does_not_exist",
		})
		require.ErrorAs(t, err, &store.ErrVercelResourceAlreadyExists{})
	})

	t.Run("a project can back only one active resource", func(t *testing.T) {
		projectID := newProject(t, "vr-one-per-project")
		newResource(t, "res_p1", projectID)

		_, err := sqlStore.CreateVercelResource(ctx, org, &store.VercelResource{
			ResourceID: "res_p2", InstallationID: "icfg_1", ProductSlug: "postgres",
			BillingPlanID: "plan_free", XataProjectID: projectID,
		})
		require.ErrorAs(t, err, &store.ErrVercelResourceProjectLinked{})
	})

	t.Run("cannot create a resource for an unavailable project", func(t *testing.T) {
		// A project owned by a different org, even though it exists.
		foreignProject, err := sqlStore.CreateProject(ctx, "other-org", createProjectConfig("vr-foreign", nil))
		require.NoError(t, err)
		_, err = sqlStore.CreateVercelResource(ctx, org, &store.VercelResource{
			ResourceID: "res_foreign", InstallationID: "icfg_1", ProductSlug: "postgres",
			BillingPlanID: "plan_free", XataProjectID: foreignProject.ID,
		})
		require.ErrorAs(t, err, &store.ErrProjectNotFound{})

		// A non-existent project.
		_, err = sqlStore.CreateVercelResource(ctx, org, &store.VercelResource{
			ResourceID: "res_noproject", InstallationID: "icfg_1", ProductSlug: "postgres",
			BillingPlanID: "plan_free", XataProjectID: "prj_does_not_exist",
		})
		require.ErrorAs(t, err, &store.ErrProjectNotFound{})

		// An in-org but terminated (inactive) project.
		inactive := newProject(t, "vr-inactive-project")
		_, err = sqlStore.sql.ExecContext(ctx,
			`UPDATE projects SET status = $1 WHERE id = $2`, StatusTerminated, inactive)
		require.NoError(t, err)
		_, err = sqlStore.CreateVercelResource(ctx, org, &store.VercelResource{
			ResourceID: "res_inactive_project", InstallationID: "icfg_1", ProductSlug: "postgres",
			BillingPlanID: "plan_free", XataProjectID: inactive,
		})
		require.ErrorAs(t, err, &store.ErrProjectNotFound{})
	})

	t.Run("scoped to the owning installation", func(t *testing.T) {
		projectID := newProject(t, "vr-tenant")
		newResource(t, "res_tenant", projectID) // owned by icfg_1

		// Another installation cannot see or mutate it.
		_, err := sqlStore.GetVercelResource(ctx, "icfg_other", "res_tenant")
		require.ErrorAs(t, err, &store.ErrVercelResourceNotFound{})
		err = sqlStore.TriggerVercelResourceDeletion(ctx, "icfg_other", "res_tenant")
		require.ErrorAs(t, err, &store.ErrVercelResourceNotFound{})
		_, err = sqlStore.AddVercelResourceBranch(ctx, "icfg_other", &store.VercelResourceBranch{
			ResourceID: "res_tenant", Scope: store.VercelScopeProduction, XataBranchID: newBranch(t, projectID, "main"),
		})
		require.ErrorAs(t, err, &store.ErrVercelResourceNotFound{})

		// The owner still can.
		got, err := sqlStore.GetVercelResource(ctx, "icfg_1", "res_tenant")
		require.NoError(t, err)
		require.Equal(t, "res_tenant", got.ResourceID)
	})

	t.Run("trigger deletion flags deleting and keeps it retrievable", func(t *testing.T) {
		newResource(t, "res_del", newProject(t, "vr-delete"))

		require.NoError(t, sqlStore.TriggerVercelResourceDeletion(ctx, "icfg_1", "res_del"))
		got, err := sqlStore.GetVercelResource(ctx, "icfg_1", "res_del")
		require.NoError(t, err)
		require.Equal(t, store.VercelResourceDeleting, got.Status)

		// A second trigger on a non-active row is refused.
		err = sqlStore.TriggerVercelResourceDeletion(ctx, "icfg_1", "res_del")
		require.ErrorAs(t, err, &store.ErrVercelResourceNotActive{})
	})

	t.Run("trigger deletion on a missing resource returns not found", func(t *testing.T) {
		err := sqlStore.TriggerVercelResourceDeletion(ctx, "icfg_1", "res_nope")
		require.ErrorAs(t, err, &store.ErrVercelResourceNotFound{})
	})

	t.Run("production and extra branches are all rows, keyed by scope", func(t *testing.T) {
		projectID := newProject(t, "vr-branches")
		newResource(t, "res_br", projectID)

		// The production/main branch is a regular row (Scope "production").
		prodID := newBranch(t, projectID, "main")
		prod, err := sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_br", Scope: store.VercelScopeProduction, XataBranchID: prodID,
		})
		require.NoError(t, err)
		require.NotEmpty(t, prod.ID)
		require.Equal(t, store.VercelResourceActive, prod.Status)

		previewID := newBranch(t, projectID, "preview")
		_, err = sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_br", Scope: store.VercelScopePreview, XataBranchID: previewID,
		})
		require.NoError(t, err)

		branches, err := sqlStore.ListVercelResourceBranches(ctx, "icfg_1", "res_br")
		require.NoError(t, err)
		require.Len(t, branches, 2)
		byScope := map[string]string{}
		for _, b := range branches {
			byScope[b.Scope] = b.XataBranchID
		}
		require.Equal(t, prodID, byScope[store.VercelScopeProduction])
		require.Equal(t, previewID, byScope[store.VercelScopePreview])

		// A second active branch for an existing scope conflicts.
		_, err = sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_br", Scope: store.VercelScopePreview, XataBranchID: newBranch(t, projectID, "preview-2"),
		})
		require.ErrorAs(t, err, &store.ErrVercelResourceBranchExists{})

		// A Xata branch can back at most one scope: reusing the production branch
		// for the development scope conflicts.
		_, err = sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_br", Scope: store.VercelScopeDevelopment, XataBranchID: prodID,
		})
		require.ErrorAs(t, err, &store.ErrVercelResourceXataBranchLinked{})

		require.NoError(t, sqlStore.TriggerVercelResourceBranchDeletion(ctx, "icfg_1", "res_br", store.VercelScopePreview))
		err = sqlStore.TriggerVercelResourceBranchDeletion(ctx, "icfg_1", "res_br", store.VercelScopePreview)
		require.ErrorAs(t, err, &store.ErrVercelResourceBranchNotActive{})

		err = sqlStore.TriggerVercelResourceBranchDeletion(ctx, "icfg_1", "res_br", store.VercelScopeDevelopment)
		require.ErrorAs(t, err, &store.ErrVercelResourceBranchNotFound{})
	})

	t.Run("cannot add a branch that is not an active branch of the project", func(t *testing.T) {
		projectID := newProject(t, "vr-owner")
		newResource(t, "res_owner", projectID)

		// A branch from another project — exists, but not this resource's project.
		otherProject := newProject(t, "vr-other")
		foreignBranch := newBranch(t, otherProject, "main")
		_, err := sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_owner", Scope: store.VercelScopeProduction, XataBranchID: foreignBranch,
		})
		require.ErrorAs(t, err, &store.ErrBranchNotFound{})

		// A non-existent branch id.
		_, err = sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_owner", Scope: store.VercelScopeProduction, XataBranchID: "br_does_not_exist",
		})
		require.ErrorAs(t, err, &store.ErrBranchNotFound{})

		// A branch of this project that is no longer active.
		inactiveBranch := newBranch(t, projectID, "to-terminate")
		_, err = sqlStore.sql.ExecContext(ctx,
			`UPDATE branches SET status = $1 WHERE id = $2`, StatusTerminated, inactiveBranch)
		require.NoError(t, err)
		_, err = sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_owner", Scope: store.VercelScopeProduction, XataBranchID: inactiveBranch,
		})
		require.ErrorAs(t, err, &store.ErrBranchNotFound{})
	})

	t.Run("hard-deleting the backing rows cascades to vercel tracking rows", func(t *testing.T) {
		// xata_branch_id → branches CASCADE: deleting the branch (as GC does) removes its tracking row.
		p1 := newProject(t, "vr-cascade-branch")
		newResource(t, "res_cascade_b", p1)
		b1 := newBranch(t, p1, "main")
		_, err := sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_cascade_b", Scope: store.VercelScopeProduction, XataBranchID: b1,
		})
		require.NoError(t, err)
		_, err = sqlStore.sql.ExecContext(ctx, `DELETE FROM branches WHERE id = $1`, b1)
		require.NoError(t, err)
		got, err := sqlStore.ListVercelResourceBranches(ctx, "icfg_1", "res_cascade_b")
		require.NoError(t, err)
		require.Empty(t, got)

		// resource_id → vercel_resources CASCADE: deleting the resource removes its branch rows.
		p2 := newProject(t, "vr-cascade-resource")
		newResource(t, "res_cascade_r", p2)
		b2 := newBranch(t, p2, "main")
		_, err = sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_cascade_r", Scope: store.VercelScopeProduction, XataBranchID: b2,
		})
		require.NoError(t, err)
		_, err = sqlStore.sql.ExecContext(ctx, `DELETE FROM vercel_resources WHERE resource_id = $1`, "res_cascade_r")
		require.NoError(t, err)
		got, err = sqlStore.ListVercelResourceBranches(ctx, "icfg_1", "res_cascade_r")
		require.NoError(t, err)
		require.Empty(t, got)

		// xata_project_id → projects CASCADE: deleting the project (as GC does) removes the resource.
		p3 := newProject(t, "vr-cascade-project")
		newResource(t, "res_cascade_p", p3)
		_, err = sqlStore.sql.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, p3)
		require.NoError(t, err)
		_, err = sqlStore.GetVercelResource(ctx, "icfg_1", "res_cascade_p")
		require.ErrorAs(t, err, &store.ErrVercelResourceNotFound{})
	})

	t.Run("cannot add a branch to a non-active resource", func(t *testing.T) {
		projectID := newProject(t, "vr-inactive")
		newResource(t, "res_inactive", projectID)
		require.NoError(t, sqlStore.TriggerVercelResourceDeletion(ctx, "icfg_1", "res_inactive"))

		_, err := sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_inactive", Scope: store.VercelScopeProduction, XataBranchID: newBranch(t, projectID, "extra"),
		})
		require.ErrorAs(t, err, &store.ErrVercelResourceNotActive{})
	})
}
