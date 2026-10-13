package reconciler

import (
	"bytes"
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"xata/internal/pgbackrest"
	"xata/services/branch-operator/api/v1alpha1"
)

// reconcilePgBackRestCipher ensures that the pgBackRest cipher Secret for the
// Branch exists with the keys its cipher references demand, and that the
// Branch owns it. A Secret that already exists is adopted without touching its
// data: the Secret is immutable and a changed passphrase would make the
// repository unreadable.
func (r *BranchReconciler) reconcilePgBackRestCipher(
	ctx context.Context,
	branch *v1alpha1.Branch,
) (controllerutil.OperationResult, error) {
	// Get the names of the keys that need to be present in the cipher Secret
	targetKey, restoreKey := pgbackrestCipherKeys(branch)

	// If the Branch neither encrypts its own repository nor restores from an
	// encrypted repository, it does not need a cipher Secret
	if targetKey == "" && restoreKey == "" {
		return controllerutil.OperationResultNone, nil
	}

	secret := &corev1.Secret{
		Name:      pgbackrest.CipherSecretName(branch.Name),
		Namespace: r.ClustersNamespace,
	}

	return controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Set the controller reference on the Secret to the Branch.
		if err := controllerutil.SetControllerReference(branch, secret, r.Scheme); err != nil {
			return err
		}

		// Ensure that labels are set on the Secret
		ensureLabels(secret, branch.Spec.InheritedMetadata)

		// Set the CNPG reload annotatation on the Secret to trigger reloads of the
		// CNPG Cluster when the Secret changes.
		secret.Labels["cnpg.io/reload"] = kubeTrue

		// A cipher must never change once a repository uses it
		secret.Immutable = new(true)

		// Only populate secret data on initial creation. If the Secret already has
		// data, preserve it
		if len(secret.Data) > 0 {
			return nil
		}

		// Refuse to generate a new cipher for a Branch that has already created
		// backups or streamed WAL: the repository would become unreadable
		if targetKey != "" && branch.HasBackupData() {
			return &ConditionError{
				ConditionType:   v1alpha1.BranchReadyConditionType,
				ConditionReason: v1alpha1.PgBackRestCipherLostReason,
				Err:             fmt.Errorf("lost cipher passphrase for repository %s", secret.Name),
			}
		}

		// Generate the data for the new Secret
		data, err := r.pgbackrestCipherData(ctx, branch, targetKey, restoreKey)
		if err != nil {
			return err
		}

		secret.Type = corev1.SecretTypeOpaque
		secret.Data = data
		return nil
	})
}

// pgbackrestCipherKeys returns the data keys the cipher Secret of the Branch
// must hold: targetKey when the Branch encrypts its own repository and
// restoreKey when it restores from an encrypted repository.
func pgbackrestCipherKeys(branch *v1alpha1.Branch) (targetKey, restoreKey string) {
	if branch.Spec.BackupSpec.IsPgBackRest() {
		if ref := branch.Spec.BackupSpec.PgBackRest.CipherPassphraseSecretRef; ref != nil {
			targetKey = ref.Key
		}
	}

	if branch.Spec.Restore.IsObjectStoreType() {
		if ref := branch.Spec.Restore.PgBackRestCipherPassphraseSecretRef; ref != nil {
			restoreKey = ref.Key
		}
	}

	return targetKey, restoreKey
}

// pgbackrestCipherData builds the data of a new cipher Secret: a generated
// passphrase under targetKey, and the cipher of the restore source under
// restoreKey.
func (r *BranchReconciler) pgbackrestCipherData(
	ctx context.Context,
	branch *v1alpha1.Branch,
	targetKey, restoreKey string,
) (map[string][]byte, error) {
	data := make(map[string][]byte, 2)

	if targetKey != "" {
		passphrase, err := pgbackrest.GeneratePassphrase()
		if err != nil {
			return nil, fmt.Errorf("generate pgbackrest cipher: %w", err)
		}
		data[targetKey] = passphrase
	}

	if restoreKey != "" {
		passphrase, err := r.restoreSourcePgBackRestCipher(ctx, branch.Spec.Restore.Name)
		if err != nil {
			return nil, err
		}
		data[restoreKey] = passphrase
	}

	return data, nil
}

// restoreSourcePgBackRestCipher returns the repository cipher of the Branch a
// restore reads from. The source Branch owns its cipher Secret and takes it
// along when deleted, so the value is copied rather than referenced: the
// restored Branch stays readable after the source is gone.
func (r *BranchReconciler) restoreSourcePgBackRestCipher(ctx context.Context, sourceName string) ([]byte, error) {
	sourceBranch := &v1alpha1.Branch{}

	// Try to get the source Branch from which the restore cipher will be copied
	err := r.Get(ctx, client.ObjectKey{
		Name: sourceName,
	}, sourceBranch)
	if apierrors.IsNotFound(err) {
		return nil, &ConditionError{
			ConditionType:   v1alpha1.BranchReadyConditionType,
			ConditionReason: v1alpha1.ParentBranchNotFoundReason,
			Err:             err,
		}
	}
	if err != nil {
		return nil, fmt.Errorf("get restore source branch %s: %w", sourceName, err)
	}

	bs := sourceBranch.Spec.BackupSpec
	if bs == nil || bs.PgBackRest == nil || bs.PgBackRest.CipherPassphraseSecretRef == nil {
		return nil, fmt.Errorf("restore source branch %s has no pgbackrest cipher", sourceName)
	}
	ref := bs.PgBackRest.CipherPassphraseSecretRef

	// Get the source cipher Secret
	sourceSecret := &corev1.Secret{}
	err = r.Get(ctx, client.ObjectKey{
		Name:      ref.Name,
		Namespace: r.ClustersNamespace,
	}, sourceSecret)
	if err != nil {
		return nil, fmt.Errorf("get restore source cipher secret %s: %w", ref.Name, err)
	}

	// Get the passphrase from the source cipher Secret
	passphrase := sourceSecret.Data[ref.Key]
	if len(passphrase) == 0 {
		return nil, fmt.Errorf("restore source cipher secret %s does not contain key %s", ref.Name, ref.Key)
	}

	return bytes.Clone(passphrase), nil
}
