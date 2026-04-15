package reconciler_test

import (
	"context"
	"testing"

	"xata/services/branch-operator/api/v1alpha1"

	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func TestNetworkPolicyReconciliation(t *testing.T) {
	t.Parallel()

	t.Run("network policy is created on branch creation", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			// Expect the NetworkPolicy to be created
			requireEventuallyNoErr(t, func() error {
				np := networkingv1.NetworkPolicy{}
				return getK8SObject(ctx, br.Name, &np)
			})
		})
	})

	t.Run("network policy is owned by the Branch", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			np := networkingv1.NetworkPolicy{}

			// Expect the NetworkPolicy to be created with the correct owner reference
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, br.Name, &np)
			})
			require.Len(t, np.GetOwnerReferences(), 1)
			require.Equal(t, br.Name, np.GetOwnerReferences()[0].Name)
		})
	})

	t.Run("network policy is deleted when cluster name is unset", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			np := networkingv1.NetworkPolicy{}

			// Expect the NetworkPolicy to be created
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, br.Name, &np)
			})

			// Remove the cluster name from the Branch
			err := retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Spec.ClusterSpec.Name = nil
			})
			require.NoError(t, err)

			// Expect the NetworkPolicy to be deleted
			requireEventuallyTrue(t, func() bool {
				err := getK8SObject(ctx, br.Name, &np)
				return apierrors.IsNotFound(err)
			})
		})
	})
}
