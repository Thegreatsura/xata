package sqlstore

import (
	"context"
	"testing"

	"xata/services/auth/store"

	"github.com/stretchr/testify/require"
)

func TestSQLAuthStoreVercelResources(t *testing.T) {
	ctx := context.Background()
	sqlStore := setupSQLStore(ctx, t)

	// xata_project_id / xata_branch_id are logical references to the projects
	// database (no FK), so tests use arbitrary ids — the store does not validate
	// their existence.
	newResource := func(t *testing.T, resourceID, projectID string) {
		t.Helper()
		_, err := sqlStore.CreateVercelResource(ctx, &store.VercelResource{
			ResourceID:     resourceID,
			InstallationID: "icfg_1",
			ProductSlug:    "postgres",
			BillingPlanID:  "plan_free",
			XataProjectID:  projectID,
		})
		require.NoError(t, err)
	}

	t.Run("create and get round-trips all fields", func(t *testing.T) {
		resource := &store.VercelResource{
			ResourceID:     "res_roundtrip",
			InstallationID: "icfg_1",
			ProductSlug:    "postgres",
			BillingPlanID:  "plan_free",
			XataProjectID:  "prj_roundtrip",
			Name:           "My DB",
			Metadata:       map[string]any{"region": "us-east-1", "instanceType": "small"},
		}
		created, err := sqlStore.CreateVercelResource(ctx, resource)
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
		require.Equal(t, "prj_roundtrip", got.XataProjectID)
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
		newResource(t, "res_dup", "prj_dup")
		// A fresh project so the collision is on the id, not the project index.
		_, err := sqlStore.CreateVercelResource(ctx, &store.VercelResource{
			ResourceID: "res_dup", InstallationID: "icfg_1", ProductSlug: "postgres",
			BillingPlanID: "plan_free", XataProjectID: "prj_dup_2",
		})
		require.ErrorAs(t, err, &store.ErrVercelResourceAlreadyExists{})
	})

	t.Run("a project can back only one active resource", func(t *testing.T) {
		newResource(t, "res_p1", "prj_shared")
		_, err := sqlStore.CreateVercelResource(ctx, &store.VercelResource{
			ResourceID: "res_p2", InstallationID: "icfg_1", ProductSlug: "postgres",
			BillingPlanID: "plan_free", XataProjectID: "prj_shared",
		})
		require.ErrorAs(t, err, &store.ErrVercelResourceProjectLinked{})
	})

	t.Run("scoped to the owning installation", func(t *testing.T) {
		newResource(t, "res_tenant", "prj_tenant") // owned by icfg_1

		_, err := sqlStore.GetVercelResource(ctx, "icfg_other", "res_tenant")
		require.ErrorAs(t, err, &store.ErrVercelResourceNotFound{})
		err = sqlStore.TriggerVercelResourceDeletion(ctx, "icfg_other", "res_tenant")
		require.ErrorAs(t, err, &store.ErrVercelResourceNotFound{})
		_, err = sqlStore.AddVercelResourceBranch(ctx, "icfg_other", &store.VercelResourceBranch{
			ResourceID: "res_tenant", Scope: store.VercelScopeProduction, XataBranchID: "br_main",
		})
		require.ErrorAs(t, err, &store.ErrVercelResourceNotFound{})

		got, err := sqlStore.GetVercelResource(ctx, "icfg_1", "res_tenant")
		require.NoError(t, err)
		require.Equal(t, "res_tenant", got.ResourceID)
	})

	t.Run("trigger deletion flags deleting and keeps it retrievable", func(t *testing.T) {
		newResource(t, "res_del", "prj_delete")

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

	t.Run("branches are all rows, keyed by scope", func(t *testing.T) {
		newResource(t, "res_br", "prj_branches")

		prod, err := sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_br", Scope: store.VercelScopeProduction, XataBranchID: "br_main",
		})
		require.NoError(t, err)
		require.NotEmpty(t, prod.ID)
		require.Equal(t, store.VercelResourceActive, prod.Status)

		_, err = sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_br", Scope: store.VercelScopePreview, XataBranchID: "br_preview",
		})
		require.NoError(t, err)

		branches, err := sqlStore.ListVercelResourceBranches(ctx, "icfg_1", "res_br")
		require.NoError(t, err)
		require.Len(t, branches, 2)
		byScope := map[string]string{}
		for _, b := range branches {
			byScope[b.Scope] = b.XataBranchID
		}
		require.Equal(t, "br_main", byScope[store.VercelScopeProduction])
		require.Equal(t, "br_preview", byScope[store.VercelScopePreview])

		// A second active branch for an existing scope conflicts.
		_, err = sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_br", Scope: store.VercelScopePreview, XataBranchID: "br_preview_2",
		})
		require.ErrorAs(t, err, &store.ErrVercelResourceBranchExists{})

		// A Xata branch backs at most one scope: reusing br_main for development conflicts.
		_, err = sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_br", Scope: store.VercelScopeDevelopment, XataBranchID: "br_main",
		})
		require.ErrorAs(t, err, &store.ErrVercelResourceXataBranchLinked{})

		require.NoError(t, sqlStore.TriggerVercelResourceBranchDeletion(ctx, "icfg_1", "res_br", store.VercelScopePreview))
		err = sqlStore.TriggerVercelResourceBranchDeletion(ctx, "icfg_1", "res_br", store.VercelScopePreview)
		require.ErrorAs(t, err, &store.ErrVercelResourceBranchNotActive{})

		err = sqlStore.TriggerVercelResourceBranchDeletion(ctx, "icfg_1", "res_br", store.VercelScopeDevelopment)
		require.ErrorAs(t, err, &store.ErrVercelResourceBranchNotFound{})
	})

	t.Run("cannot add a branch to a non-active resource", func(t *testing.T) {
		newResource(t, "res_inactive", "prj_inactive")
		require.NoError(t, sqlStore.TriggerVercelResourceDeletion(ctx, "icfg_1", "res_inactive"))

		_, err := sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_inactive", Scope: store.VercelScopeProduction, XataBranchID: "br_extra",
		})
		require.ErrorAs(t, err, &store.ErrVercelResourceNotActive{})
	})

	t.Run("deleting a resource row cascades to its branch rows", func(t *testing.T) {
		// resource_id → vercel_resources is an intra-database FK with ON DELETE CASCADE.
		newResource(t, "res_cascade", "prj_cascade")
		_, err := sqlStore.AddVercelResourceBranch(ctx, "icfg_1", &store.VercelResourceBranch{
			ResourceID: "res_cascade", Scope: store.VercelScopeProduction, XataBranchID: "br_cascade",
		})
		require.NoError(t, err)
		_, err = sqlStore.sql.ExecContext(ctx, `DELETE FROM vercel_resources WHERE resource_id = $1`, "res_cascade")
		require.NoError(t, err)
		got, err := sqlStore.ListVercelResourceBranches(ctx, "icfg_1", "res_cascade")
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("create resource and branches persists the resource and all rows", func(t *testing.T) {
		created, err := sqlStore.CreateVercelResourceAndBranches(ctx,
			&store.VercelResource{ResourceID: "res_atomic", InstallationID: "icfg_1", ProductSlug: "postgres", XataProjectID: "prj_atomic", Name: "atomic"},
			[]store.VercelResourceBranch{
				{ResourceID: "res_atomic", Scope: store.VercelScopeProduction, XataBranchID: "br_atomic_main"},
				{ResourceID: "res_atomic", Scope: store.VercelScopeDevelopment, XataBranchID: "br_atomic_dev"},
			},
		)
		require.NoError(t, err)
		require.Equal(t, "res_atomic", created.ResourceID)
		require.Equal(t, store.VercelResourceActive, created.Status)

		branches, err := sqlStore.ListVercelResourceBranches(ctx, "icfg_1", "res_atomic")
		require.NoError(t, err)
		require.Len(t, branches, 2)
	})

	t.Run("create resource and branches rolls back when a branch row is invalid", func(t *testing.T) {
		// Two branch rows sharing a xata_branch_id violate the unique index, failing
		// mid-transaction, so the whole write — including the resource row — rolls back.
		_, err := sqlStore.CreateVercelResourceAndBranches(ctx,
			&store.VercelResource{ResourceID: "res_rollback", InstallationID: "icfg_1", ProductSlug: "postgres", XataProjectID: "prj_rollback", Name: "rollback"},
			[]store.VercelResourceBranch{
				{ResourceID: "res_rollback", Scope: store.VercelScopeProduction, XataBranchID: "br_dupe"},
				{ResourceID: "res_rollback", Scope: store.VercelScopeDevelopment, XataBranchID: "br_dupe"},
			},
		)
		require.Error(t, err)

		_, err = sqlStore.GetVercelResource(ctx, "icfg_1", "res_rollback")
		require.ErrorAs(t, err, &store.ErrVercelResourceNotFound{})
	})

	t.Run("resource lock is non-blocking: contention returns busy, release frees it", func(t *testing.T) {
		const resourceID = "res_lock"

		release, err := sqlStore.TryAcquireVercelResourceLock(ctx, resourceID)
		require.NoError(t, err)

		// A second attempt while held fails fast with busy — it does not block.
		_, err = sqlStore.TryAcquireVercelResourceLock(ctx, resourceID)
		require.ErrorAs(t, err, &store.ErrVercelResourceBusy{})

		// A different resource id is not contended.
		otherRelease, err := sqlStore.TryAcquireVercelResourceLock(ctx, "res_lock_other")
		require.NoError(t, err)
		require.NoError(t, otherRelease())

		// Once released, it can be acquired again.
		require.NoError(t, release())
		again, err := sqlStore.TryAcquireVercelResourceLock(ctx, resourceID)
		require.NoError(t, err)
		require.NoError(t, again())
	})
}
