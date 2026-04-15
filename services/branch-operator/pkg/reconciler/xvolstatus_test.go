package reconciler_test

import (
	"context"
	"testing"

	"xata/services/branch-operator/api/v1alpha1"

	"github.com/stretchr/testify/require"
	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestXVolStatus(t *testing.T) {
	t.Parallel()

	t.Run("XVol info is unavailable when branch has no cluster", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		// Create a Branch with no associated Cluster
		branch := NewBranchBuilder().
			WithClusterName(nil).
			Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			// Wait for the XVolInfoAvailable condition to be set
			requireEventuallyTrue(t, func() bool {
				err := getK8SObject(ctx, br.Name, br)
				if err != nil {
					return false
				}
				c := meta.FindStatusCondition(br.Status.Conditions, v1alpha1.XVolInfoAvailableConditionType)
				if c == nil {
					return false
				}
				return c.Status != metav1.ConditionUnknown
			})

			// Expect XVolInfoAvailable to be False because the Branch has no
			// Cluster associated with it
			c := meta.FindStatusCondition(br.Status.Conditions, v1alpha1.XVolInfoAvailableConditionType)
			require.NotNil(t, c)
			require.Equal(t, metav1.ConditionFalse, c.Status)
			require.Equal(t, v1alpha1.BranchHasNoClusterReason, c.Reason)

			// Assert PrimaryXVolName is empty
			require.Empty(t, br.Status.PrimaryXVolName)
		})
	})

	t.Run("XVol info available when Cluster exists", func(t *testing.T) {
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

			xvolName, pvcName, _ := createPVCAndXVol(ctx, t, clusterName)

			// Set the Cluster's CurrentPrimary so getClusterPVC resolves
			setClusterStatus(ctx, t, &cluster, apiv1.ClusterStatus{
				CurrentPrimary: pvcName,
			})

			// Trigger re-reconciliation by updating a spec field
			err := retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Spec.ClusterSpec.Instances = 2
			})
			require.NoError(t, err)

			// Assert PrimaryXVolName is set and XVolInfoAvailable is True
			requireEventuallyTrue(t, func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(br), br)
				if err != nil {
					return false
				}
				return br.Status.PrimaryXVolName == xvolName &&
					meta.IsStatusConditionTrue(br.Status.Conditions, v1alpha1.XVolInfoAvailableConditionType)
			})
		})
	})

	t.Run("XVol info retained when cluster name is removed", func(t *testing.T) {
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

			xvolName, pvcName, _ := createPVCAndXVol(ctx, t, clusterName)

			// Set the Cluster's CurrentPrimary so getClusterPVC resolves
			setClusterStatus(ctx, t, &cluster, apiv1.ClusterStatus{
				CurrentPrimary: pvcName,
			})

			// Trigger re-reconciliation by updating a spec field
			err := retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Spec.ClusterSpec.Instances = 2
			})
			require.NoError(t, err)

			// Wait for PrimaryXVolName to be set
			requireEventuallyTrue(t, func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(br), br)
				if err != nil {
					return false
				}
				return br.Status.PrimaryXVolName == xvolName
			})

			// Remove the cluster name from the branch
			err = retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Spec.ClusterSpec.Name = nil
			})
			require.NoError(t, err)

			// Assert XVolInfoAvailable becomes False with BranchHasNoCluster reason,
			// but PrimaryXVolName is retained
			requireEventuallyTrue(t, func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(br), br)
				if err != nil {
					return false
				}
				c := meta.FindStatusCondition(br.Status.Conditions, v1alpha1.XVolInfoAvailableConditionType)
				if c == nil {
					return false
				}
				return c.Status == metav1.ConditionFalse &&
					c.Reason == v1alpha1.BranchHasNoClusterReason &&
					br.Status.PrimaryXVolName == xvolName
			})
		})
	})
}
