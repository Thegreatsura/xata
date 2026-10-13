package reconciler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"xata/internal/pgbackrest"
	"xata/services/branch-operator/api/v1alpha1"
)

func TestPgBackRestCipherReconciliation(t *testing.T) {
	t.Parallel()

	t.Run("pre-existing secret is adopted and its data preserved", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branchName := randomString(10)

		// Create a cipher Secret before the branch exists
		secret := &corev1.Secret{
			Name:      pgbackrest.CipherSecretName(branchName),
			Namespace: XataClustersNamespace,
			Immutable: new(true),
			Type:      corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				pgbackrest.CipherPassphraseKey: []byte("original-passphrase"),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, secret))

		br := NewBranchBuilder().
			WithName(branchName).
			WithPgBackRest("some-bucket", "us-east-1").
			WithPgBackRestCipher().
			Build()

		withBranch(ctx, t, br, func(t *testing.T, br *v1alpha1.Branch) {
			// Wait for reconciliation to adopt the Secret
			var got corev1.Secret
			requireEventuallyTrue(t, func() bool {
				if err := getK8SObject(ctx, secret.Name, &got); err != nil {
					return false
				}
				return len(got.GetOwnerReferences()) == 1
			})

			require.Equal(t, br.Name, got.GetOwnerReferences()[0].Name)
			require.Equal(t, "original-passphrase", string(got.Data[pgbackrest.CipherPassphraseKey]))
			require.Equal(t, new(true), got.Immutable)
		})
	})

	t.Run("secret is created with a generated passphrase", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		br := NewBranchBuilder().
			WithPgBackRest("some-bucket", "us-east-1").
			WithPgBackRestCipher().
			Build()

		withBranch(ctx, t, br, func(t *testing.T, br *v1alpha1.Branch) {
			var got corev1.Secret
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, pgbackrest.CipherSecretName(br.Name), &got)
			})

			require.Equal(t, new(true), got.Immutable)
			require.Equal(t, corev1.SecretTypeOpaque, got.Type)
			require.NotEmpty(t, got.Data[pgbackrest.CipherPassphraseKey])
			require.NotContains(t, got.Data, pgbackrest.RestoreCipherPassphraseKey)
			require.Equal(t, "true", got.Labels["cnpg.io/reload"])
			require.Len(t, got.GetOwnerReferences(), 1)
			require.Equal(t, br.Name, got.GetOwnerReferences()[0].Name)
		})
	})

	t.Run("restore from an encrypted source copies its passphrase", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		// The source branch and its cipher Secret exist before the restore
		sourceName := randomString(10)
		sourceSecret := &corev1.Secret{
			Name:      pgbackrest.CipherSecretName(sourceName),
			Namespace: XataClustersNamespace,
			Immutable: new(true),
			Type:      corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				pgbackrest.CipherPassphraseKey: []byte("source-passphrase"),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, sourceSecret))

		// Create the parent branch with pgBackRest and a cipher
		source := NewBranchBuilder().
			WithName(sourceName).
			WithPgBackRest("some-bucket", "us-east-1").
			WithPgBackRestCipher().
			Build()
		require.NoError(t, k8sClient.Create(ctx, &source))

		// Create a new branch that restores from the source branch
		br := NewBranchBuilder().
			WithPgBackRest("some-bucket", "us-east-1").
			WithPgBackRestCipher().
			WithPgBackRestRestoreCipher(sourceName).
			Build()

		withBranch(ctx, t, br, func(t *testing.T, br *v1alpha1.Branch) {
			var got corev1.Secret

			// Wait for the cipher secret for the child branch to be created
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, pgbackrest.CipherSecretName(br.Name), &got)
			})

			// The child branch's cipher secret should have the source passphrase
			// under the restore key
			require.Equal(t, "source-passphrase", string(got.Data[pgbackrest.RestoreCipherPassphraseKey]))
			require.NotEmpty(t, got.Data[pgbackrest.CipherPassphraseKey])
		})
	})

	t.Run("lost secret is not regenerated once the repository has data", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		br := NewBranchBuilder().
			WithPgBackRest("some-bucket", "us-east-1").
			WithPgBackRestCipher().
			Build()

		withBranch(ctx, t, br, func(t *testing.T, br *v1alpha1.Branch) {
			secretName := pgbackrest.CipherSecretName(br.Name)

			// Wait for the cipher secret to be created
			var secret corev1.Secret
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, secretName, &secret)
			})

			// Record a backup on the branch, then lose the secret. The deletion
			// triggers a reconcile of the owning branch.
			err := retryStatusOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Status.LastSuccessfulBackup = "2026-10-12T00:00:00Z"
			})
			require.NoError(t, err)
			require.NoError(t, k8sClient.Delete(ctx, &secret))

			// Expect the branch to fail reconciliation due to the lost cipher secret
			// and report the fact via status
			requireEventuallyTrue(t, func() bool {
				if err := getK8SObject(ctx, br.Name, br); err != nil {
					return false
				}

				c := meta.FindStatusCondition(br.Status.Conditions, v1alpha1.BranchReadyConditionType)
				if c == nil {
					return false
				}

				return c.Status == metav1.ConditionFalse && c.Reason == v1alpha1.PgBackRestCipherLostReason
			})

			// Assert that the secret has not been recreated
			err = getK8SObject(ctx, secretName, &corev1.Secret{})
			require.True(t, apierrors.IsNotFound(err))
		})
	})

	t.Run("restore from a missing source branch fails with a status condition error", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		br := NewBranchBuilder().
			WithPgBackRest("some-bucket", "us-east-1").
			WithPgBackRestCipher().
			WithPgBackRestRestoreCipher("does-not-exist").
			Build()

		withBranch(ctx, t, br, func(t *testing.T, br *v1alpha1.Branch) {
			// Expect the branch to report the missing source
			requireEventuallyTrue(t, func() bool {
				if err := getK8SObject(ctx, br.Name, br); err != nil {
					return false
				}

				c := meta.FindStatusCondition(br.Status.Conditions, v1alpha1.BranchReadyConditionType)
				if c == nil {
					return false
				}

				return c.Status == metav1.ConditionFalse && c.Reason == v1alpha1.ParentBranchNotFoundReason
			})

			// Assert that no cipher secret was created
			err := getK8SObject(ctx, pgbackrest.CipherSecretName(br.Name), &corev1.Secret{})
			require.True(t, apierrors.IsNotFound(err))
		})
	})
}
