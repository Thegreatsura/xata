package reconciler_test

import (
	"context"
	"testing"

	"xata/services/branch-operator/api/v1alpha1"

	"github.com/stretchr/testify/require"
	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestBackupStatus(t *testing.T) {
	t.Parallel()

	t.Run("backup fields are copied from the Cluster status", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			clusterName := br.Name

			// Wait for the reconciler to create the CNPG Cluster
			cluster := apiv1.Cluster{}
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, clusterName, &cluster)
			})

			// Set backup fields on the Cluster status
			setClusterStatus(ctx, t, &cluster, apiv1.ClusterStatus{
				FirstRecoverabilityPoint: "2026-08-13T10:00:00Z",
				LastSuccessfulBackup:     "2026-08-13T11:00:00Z",
				LastFailedBackup:         "2026-08-13T09:00:00Z",
				LastRecoverabilityPoint:  "2026-08-13T11:30:00Z",
			})

			// Trigger re-reconciliation by updating a spec field
			err := retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Spec.ClusterSpec.SmartShutdownTimeout = new(int32(60))
			})
			require.NoError(t, err)

			// Assert the backup fields are copied to the Branch status
			requireEventuallyTrue(t, func() bool {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(br), br); err != nil {
					return false
				}
				return br.Status.FirstRecoverabilityPoint == "2026-08-13T10:00:00Z" &&
					br.Status.LastSuccessfulBackup == "2026-08-13T11:00:00Z" &&
					br.Status.LastFailedBackup == "2026-08-13T09:00:00Z" &&
					br.Status.LastRecoverabilityPoint == "2026-08-13T11:30:00Z"
			})
		})
	})

	t.Run("a backup status change alone triggers the copy", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			clusterName := br.Name

			// Wait for the reconciler to create the CNPG Cluster
			cluster := apiv1.Cluster{}
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, clusterName, &cluster)
			})

			// Set backup fields on the Cluster status; no spec change follows,
			// so only the Cluster watch predicate can trigger the reconcile
			setClusterStatus(ctx, t, &cluster, apiv1.ClusterStatus{
				LastSuccessfulBackup: "2026-08-13T11:00:00Z",
			})

			// Assert the backup field is copied to the Branch status
			requireEventuallyTrue(t, func() bool {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(br), br); err != nil {
					return false
				}
				return br.Status.LastSuccessfulBackup == "2026-08-13T11:00:00Z"
			})
		})
	})

	t.Run("backup fields are retained when the cluster name is removed", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().
			WithWakeupPool("pg18-tiny").
			Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			clusterName := br.Name

			// Wait for the reconciler to create the CNPG Cluster
			cluster := apiv1.Cluster{}
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, clusterName, &cluster)
			})

			// The XVol ownership step needs a primary PVC/XVol to retain
			_, pvcName, _ := createPVCAndXVol(ctx, t, clusterName)

			// Set backup fields and the primary on the Cluster status
			setClusterStatus(ctx, t, &cluster, apiv1.ClusterStatus{
				CurrentPrimary:       pvcName,
				LastSuccessfulBackup: "2026-08-13T11:00:00Z",
			})

			// Trigger re-reconciliation by updating a spec field
			err := retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Spec.ClusterSpec.SmartShutdownTimeout = new(int32(60))
			})
			require.NoError(t, err)

			// Wait for the backup field to be set
			requireEventuallyTrue(t, func() bool {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(br), br); err != nil {
					return false
				}
				return br.Status.LastSuccessfulBackup == "2026-08-13T11:00:00Z"
			})

			// Remove the cluster name from the branch (pool hibernation)
			err = retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Spec.ClusterSpec.Name = nil
			})
			require.NoError(t, err)

			// Wait for the Branch to be reconciled again
			requireEventuallyTrue(t, func() bool {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(br), br); err != nil {
					return false
				}
				return br.Status.ObservedGeneration == br.Generation
			})

			// The backup field is retained after the cluster is gone
			require.Equal(t, "2026-08-13T11:00:00Z", br.Status.LastSuccessfulBackup)
		})
	})
}
