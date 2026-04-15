package reconciler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"

	"xata/services/branch-operator/api/v1alpha1"
)

func TestAdditionalServicesReconciliation(t *testing.T) {
	t.Parallel()

	t.Run("additional services are created on branch creation", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			suffixes := []string{"-rw", "-r", "-ro"}

			for _, suffix := range suffixes {
				svcName := "branch-" + br.Name + suffix

				requireEventuallyNoErr(t, func() error {
					return getK8SObject(ctx, svcName, &v1.Service{})
				})
			}
		})
	})

	t.Run("additional services are owned by the Branch", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			suffixes := []string{"-rw", "-r", "-ro"}

			for _, suffix := range suffixes {
				svc := v1.Service{}
				svcName := "branch-" + br.Name + suffix

				requireEventuallyNoErr(t, func() error {
					return getK8SObject(ctx, svcName, &svc)
				})
				require.Len(t, svc.GetOwnerReferences(), 1)
				require.Equal(t, br.Name, svc.GetOwnerReferences()[0].Name)
			}
		})
	})
}
