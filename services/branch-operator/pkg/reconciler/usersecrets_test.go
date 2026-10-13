package reconciler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	corev1 "k8s.io/api/core/v1"

	"xata/services/branch-operator/api/v1alpha1"
	"xata/services/branch-operator/pkg/reconciler/resources"
)

func TestSecretsReconciliation(t *testing.T) {
	t.Parallel()

	t.Run("secrets are created on branch creation", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		secrets := []struct {
			suffix   string
			username string
		}{
			{suffix: "-superuser", username: "postgres"},
			{suffix: "-app", username: "xata"},
		}

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			for _, s := range secrets {
				secret := corev1.Secret{}
				secretName := br.Name + s.suffix

				// Expect the secret to be created
				requireEventuallyNoErr(t, func() error {
					return getK8SObject(ctx, secretName, &secret)
				})

				// Verify secret type and username
				require.Equal(t, corev1.SecretTypeBasicAuth, secret.Type)
				require.Equal(t, s.username, string(secret.Data[corev1.BasicAuthUsernameKey]))
				require.NotEmpty(t, secret.Data[corev1.BasicAuthPasswordKey])
			}
		})
	})

	t.Run("secrets are owned by the Branch", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		suffixes := []string{"-superuser", "-app"}

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			for _, suffix := range suffixes {
				secret := corev1.Secret{}

				// Expect the secret to be created
				requireEventuallyNoErr(t, func() error {
					return getK8SObject(ctx, br.Name+suffix, &secret)
				})

				require.Len(t, secret.GetOwnerReferences(), 1)
				require.Equal(t, br.Name, secret.GetOwnerReferences()[0].Name)
			}
		})
	})

	t.Run("pre-existing secrets are not modified", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		secrets := []struct {
			suffix   string
			username string
			password string
		}{
			{suffix: "-superuser", username: "postgres", password: "original-superuser-pw"},
			{suffix: "-app", username: "xata", password: "original-app-pw"},
		}

		branchName := randomString(10)

		// Pre-create the secrets before the branch exists
		for _, s := range secrets {
			secret := resources.Secret(branchName+s.suffix, XataClustersNamespace, s.username, s.password)
			require.NoError(t, k8sClient.Create(ctx, secret))
		}

		br := NewBranchBuilder().
			WithName(branchName).
			WithClusterName(new(branchName)).
			Build()

		withBranch(ctx, t, br, func(t *testing.T, br *v1alpha1.Branch) {
			// Wait for reconciliation to create the CNPG Cluster
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, br.Name, &apiv1.Cluster{})
			})

			// Verify the secrets still have their original passwords
			for _, s := range secrets {
				var secret corev1.Secret
				require.NoError(t, getK8SObject(ctx, br.Name+s.suffix, &secret))
				require.Equal(t, s.password, string(secret.Data[corev1.BasicAuthPasswordKey]))
			}
		})
	})
}
