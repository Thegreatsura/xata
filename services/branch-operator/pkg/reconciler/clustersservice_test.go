package reconciler_test

import (
	"context"
	"testing"

	"xata/services/branch-operator/api/v1alpha1"
	"xata/services/branch-operator/pkg/reconciler"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

func TestClustersServiceReconciliation(t *testing.T) {
	t.Parallel()

	t.Run("clusters service is created on branch creation", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			svcName := reconciler.ClustersServiceNamePrefix + br.Name

			// Expect the Service to be created
			requireEventuallyNoErr(t, func() error {
				svc := v1.Service{}
				return getK8SObjectInNamespace(ctx, svcName, XataNamespace, &svc)
			})
		})
	})

	t.Run("clusters service is owned by the Branch", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			svc := v1.Service{}
			svcName := reconciler.ClustersServiceNamePrefix + br.Name

			// Expect the Service to be created with the correct owner reference
			requireEventuallyNoErr(t, func() error {
				return getK8SObjectInNamespace(ctx, svcName, XataNamespace, &svc)
			})
			require.Len(t, svc.GetOwnerReferences(), 1)
			require.Equal(t, br.Name, svc.GetOwnerReferences()[0].Name)
		})
	})
}
