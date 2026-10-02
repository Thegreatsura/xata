package reconciler_test

import (
	"context"
	"testing"
	"time"

	"xata/services/branch-operator/api/v1alpha1"
	"xata/services/branch-operator/pkg/reconciler"

	"github.com/stretchr/testify/require"
	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func TestPgBackRestIdentityReconciliation(t *testing.T) {
	t.Parallel()

	t.Run("service account and token are created for AWS IAM backups", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().WithPgBackRest("some-bucket", "us-east-1").Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			sa := corev1.ServiceAccount{}
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, reconciler.PgBackRestServiceAccountName(br.Name), &sa)
			})
			require.Equal(t, br.Name+"-pgbackrest", sa.Name)
			require.Equal(t, br.ClusterName()+"-pgbackrest-web-identity", reconciler.PgBackRestWebIdentitySecretName(br.ClusterName()))
			require.Len(t, sa.GetOwnerReferences(), 1)
			require.Equal(t, br.Name, sa.GetOwnerReferences()[0].Name)

			secret := corev1.Secret{}
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, reconciler.PgBackRestWebIdentitySecretName(br.ClusterName()), &secret)
			})
			require.NotEmpty(t, secret.Data[reconciler.PgBackRestTokenKey])
			require.Contains(t, secret.Annotations[reconciler.PgBackRestTokenRequestAnnotation], "serviceAccount="+sa.Name+",")
			require.Contains(t, secret.Annotations[reconciler.PgBackRestTokenRequestAnnotation], "audiences=[sts.amazonaws.com]")

			expiresAt, err := time.Parse(time.RFC3339, secret.Annotations[reconciler.PgBackRestTokenExpirationAnnotation])
			require.NoError(t, err)
			refreshAt, err := time.Parse(time.RFC3339, secret.Annotations[reconciler.PgBackRestTokenRefreshAnnotation])
			require.NoError(t, err)
			require.True(t, refreshAt.Before(expiresAt))
		})
	})

	t.Run("nothing is created without pgbackrest backups", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			// The pgbackrest identity is reconciled before the Cluster
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, br.ClusterName(), &apiv1.Cluster{})
			})

			err := getK8SObject(ctx, reconciler.PgBackRestServiceAccountName(br.Name), &corev1.ServiceAccount{})
			require.True(t, apierrors.IsNotFound(err))
			err = getK8SObject(ctx, reconciler.PgBackRestWebIdentitySecretName(br.ClusterName()), &corev1.Secret{})
			require.True(t, apierrors.IsNotFound(err))
		})
	})

	t.Run("Secret is owned by the Cluster and the ServiceAccount by the Branch", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().WithPgBackRest("some-bucket", "us-east-1").Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			secret := corev1.Secret{}
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, reconciler.PgBackRestWebIdentitySecretName(br.ClusterName()), &secret)
			})
			cluster := apiv1.Cluster{}
			require.NoError(t, getK8SObject(ctx, br.ClusterName(), &cluster))

			// envtest runs no garbage collector, so check the owner reference
			// that makes Kubernetes delete the Secret with the Cluster.
			require.Len(t, secret.GetOwnerReferences(), 1)
			owner := secret.GetOwnerReferences()[0]
			require.Equal(t, "Cluster", owner.Kind)
			require.Equal(t, cluster.UID, owner.UID)
			require.Nil(t, owner.Controller)

			// Remove the cluster from the Branch
			err := retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Spec.ClusterSpec.Name = nil
			})
			require.NoError(t, err)

			requireEventuallyTrue(t, func() bool {
				return apierrors.IsNotFound(getK8SObject(ctx, cluster.Name, &apiv1.Cluster{}))
			})
			require.NoError(t, getK8SObject(ctx, reconciler.PgBackRestServiceAccountName(br.Name), &corev1.ServiceAccount{}))
		})
	})
}
