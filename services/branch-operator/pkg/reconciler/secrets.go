package reconciler

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"xata/internal/passwords"
	"xata/services/branch-operator/api/v1alpha1"
)

// reconcileSuperuserSecret ensures that the superuser secret exists for the
// given Branch.
func (r *BranchReconciler) reconcileSuperuserSecret(ctx context.Context,
	branch *v1alpha1.Branch,
) (controllerutil.OperationResult, error) {
	return r.reconcileSecret(ctx, branch, branch.Name+"-superuser", "postgres")
}

// reconcileAppSecret ensures that the app user (xata) secret exists for the
// given Branch.
func (r *BranchReconciler) reconcileAppSecret(ctx context.Context,
	branch *v1alpha1.Branch,
) (controllerutil.OperationResult, error) {
	return r.reconcileSecret(ctx, branch, branch.Name+"-app", "xata")
}

// reconcilePgBackRestSecrets adopts the cipher Secret references already
// created by the Clusters service. It never creates or changes secret data.
func (r *BranchReconciler) reconcilePgBackRestSecrets(
	ctx context.Context,
	branch *v1alpha1.Branch,
) (controllerutil.OperationResult, error) {
	names := pgbackrestSecretNames(branch)
	result := controllerutil.OperationResultNone

	for _, name := range names {
		expectedName := branch.Name + "-pgbackrest"
		if name != expectedName {
			return result, fmt.Errorf("pgbackrest cipher secret must be named %s, got %s", expectedName, name)
		}

		secret := &corev1.Secret{}
		key := client.ObjectKey{Name: name, Namespace: r.ClustersNamespace}
		if err := r.Get(ctx, key, secret); err != nil {
			return result, fmt.Errorf("get pgbackrest cipher secret %s: %w", name, err)
		}

		if secret.Immutable == nil || !*secret.Immutable {
			return result, fmt.Errorf("pgbackrest cipher secret %s is not immutable", name)
		}

		original := secret.DeepCopy()
		if err := controllerutil.SetControllerReference(branch, secret, r.Scheme); err != nil {
			return result, err
		}
		ensureLabels(secret, branch.Spec.InheritedMetadata)
		secret.Labels["cnpg.io/reload"] = kubeTrue
		if equality.Semantic.DeepEqual(original, secret) {
			continue
		}

		if err := r.Patch(ctx, secret, client.MergeFrom(original)); err != nil {
			return result, fmt.Errorf("adopt pgbackrest cipher secret %s: %w", name, err)
		}
		result = controllerutil.OperationResultUpdated
	}

	return result, nil
}

func pgbackrestSecretNames(branch *v1alpha1.Branch) []string {
	names := map[string]struct{}{}
	if branch.Spec.BackupSpec != nil && branch.Spec.BackupSpec.PgBackRest != nil {
		if ref := branch.Spec.BackupSpec.PgBackRest.CipherPassphraseSecretRef; ref != nil && ref.Name != "" {
			names[ref.Name] = struct{}{}
		}
	}
	if branch.Spec.Restore != nil {
		if ref := branch.Spec.Restore.PgBackRestCipherPassphraseSecretRef; ref != nil && ref.Name != "" {
			names[ref.Name] = struct{}{}
		}
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func (r *BranchReconciler) reconcileSecret(ctx context.Context,
	branch *v1alpha1.Branch, name, username string,
) (controllerutil.OperationResult, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.ClustersNamespace,
		},
	}

	return controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// TODO: Remove once all secrets have been migrated to Branch ownership.
		// Clear any existing owner references (e.g. from the CNPG Cluster)
		// so the Branch can safely take controller ownership.
		secret.OwnerReferences = nil

		// Set the controller reference on the Secret to the Branch.
		if err := controllerutil.SetControllerReference(branch, secret, r.Scheme); err != nil {
			return err
		}

		// Ensure that labels are set on the Secret
		ensureLabels(secret, branch.Spec.InheritedMetadata)

		// Set the CNPG reload annotatation on the Secret to trigger reloads of the
		// CNPG Cluster when the Secret changes.
		secret.Labels["cnpg.io/reload"] = kubeTrue

		// Only populate secret data on initial creation. If the secret
		// already has data, preserve it to avoid overwriting passwords.
		if len(secret.Data) > 0 {
			return nil
		}

		// Generate the password using the logic as CNPG (1.28)
		pw, err := passwords.Generate()
		if err != nil {
			return fmt.Errorf("generate password: %w", err)
		}

		// Set the secret type and data
		secret.Type = corev1.SecretTypeBasicAuth
		secret.Data = map[string][]byte{
			corev1.BasicAuthUsernameKey: []byte(username),
			corev1.BasicAuthPasswordKey: []byte(pw),
		}

		return nil
	})
}
