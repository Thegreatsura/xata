package clusters

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clustersv1 "xata/gen/proto/clusters/v1"
	"xata/internal/passwords"
	branchv1alpha1 "xata/services/branch-operator/api/v1alpha1"
)

const (
	pgBackRestCipherBytes  = 32
	PgBackRestSecretSuffix = "-pgbackrest"
	// PgBackRestCipherPassphraseKey is a Secret data key, not a credential.
	//nolint:gosec
	PgBackRestCipherPassphraseKey  = "cipher-passphrase"
	PgBackRestRestorePassphraseKey = "restore-cipher-passphrase"
)

// createAppSecret creates the kubernetes.io/basic-auth Secret holding the
// xata role credentials for a new branch, with a freshly generated password.
// If the secret already exists (a retried CreatePostgresCluster call, or a
// leftover from a failed attempt), its data is preserved — a password is
// never overwritten — and nil is returned so the caller does not clean it up
// on failure. The branch-operator adopts the secret (Branch ownership) on the
// first reconcile.
func (c *ClustersService) createAppSecret(ctx context.Context, name string, req *clustersv1.CreatePostgresClusterRequest) (*corev1.Secret, error) {
	pw, err := passwords.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate password: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.config.ClustersNamespace,
			Labels: map[string]string{
				// Trigger reloads of the CNPG Cluster when the Secret changes
				"cnpg.io/reload": "true",
				LabelOrgID:       req.GetOrganizationId(),
				LabelProjectID:   req.GetProjectId(),
				LabelBranchID:    req.GetId(),
			},
		},
		Type: corev1.SecretTypeBasicAuth,
		Data: map[string][]byte{
			corev1.BasicAuthUsernameKey: []byte("xata"),
			corev1.BasicAuthPasswordKey: []byte(pw),
		},
	}

	if err := c.kubeClient.Create(ctx, secret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, nil
		}
		return nil, k8sErrorToGRPCError(err)
	}
	return secret, nil
}

// createPgBackRestSecret creates the cipher Secret for a new
// pgBackRest repository when target encryption is enabled or an encrypted
// restore source requires a cipher. It updates only the prospective Branch.
// Existing Secrets are checked and reused so retries cannot change a
// repository cipher or its encryption decision.
func (c *ClustersService) createPgBackRestSecret(
	ctx context.Context,
	branch *branchv1alpha1.Branch,
	encryptTarget bool,
) (*corev1.Secret, error) {
	if !branch.Spec.BackupSpec.IsPgBackRest() {
		return nil, nil
	}

	restorePassphrase, err := c.pgBackRestRestorePassphrase(ctx, branch.Spec.Restore)
	if err != nil {
		return nil, err
	}

	key := client.ObjectKey{
		Name:      branch.Name + PgBackRestSecretSuffix,
		Namespace: c.config.ClustersNamespace,
	}
	existing := &corev1.Secret{}
	if err := c.kubeClient.Get(ctx, key, existing); err == nil {
		hasTarget, err := checkPgBackRestSecret(existing, encryptTarget, restorePassphrase)
		if err != nil {
			return nil, err
		}
		setPgBackRestCipherReferences(branch, hasTarget, restorePassphrase != nil)
		return nil, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, k8sErrorToGRPCError(err)
	}

	if !encryptTarget && restorePassphrase == nil {
		return nil, nil
	}

	data := make(map[string][]byte, 2)
	if encryptTarget {
		targetPassphrase, err := generatePgBackRestCipherPassphrase()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "generate pgbackrest cipher: %v", err)
		}
		data[PgBackRestCipherPassphraseKey] = targetPassphrase
	}
	if restorePassphrase != nil {
		data[PgBackRestRestorePassphraseKey] = restorePassphrase
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: c.config.ClustersNamespace,
			Labels: map[string]string{
				"cnpg.io/reload": "true",
				LabelOrgID:       branch.Labels[LabelOrgID],
				LabelProjectID:   branch.Labels[LabelProjectID],
				LabelBranchID:    branch.Name,
			},
		},
		Immutable: new(true),
		Type:      corev1.SecretTypeOpaque,
		Data:      data,
	}

	if err := c.kubeClient.Create(ctx, secret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if getErr := c.kubeClient.Get(ctx, client.ObjectKeyFromObject(secret), existing); getErr != nil {
				return nil, k8sErrorToGRPCError(getErr)
			}
			hasTarget, err := checkPgBackRestSecret(existing, encryptTarget, restorePassphrase)
			if err != nil {
				return nil, err
			}
			setPgBackRestCipherReferences(branch, hasTarget, restorePassphrase != nil)
			return nil, nil
		}
		return nil, k8sErrorToGRPCError(err)
	}
	setPgBackRestCipherReferences(branch, encryptTarget, restorePassphrase != nil)
	return secret, nil
}

func (c *ClustersService) pgBackRestRestorePassphrase(ctx context.Context, restore *branchv1alpha1.RestoreSpec) ([]byte, error) {
	if restore == nil || restore.Type != branchv1alpha1.RestoreTypeObjectStore {
		return nil, nil
	}

	source, err := c.getBranch(ctx, restore.Name)
	if err != nil {
		return nil, k8sErrorToGRPCError(err)
	}
	if source.Spec.BackupSpec == nil || source.Spec.BackupSpec.PgBackRest == nil ||
		source.Spec.BackupSpec.PgBackRest.CipherPassphraseSecretRef == nil {
		return nil, nil
	}

	ref := source.Spec.BackupSpec.PgBackRest.CipherPassphraseSecretRef
	if ref.Name == "" || ref.Key == "" {
		return nil, status.Errorf(codes.FailedPrecondition, "source branch %s has an invalid pgbackrest cipher reference", source.Name)
	}

	secret := &corev1.Secret{}
	if err := c.kubeClient.Get(ctx, client.ObjectKey{
		Name:      ref.Name,
		Namespace: c.config.ClustersNamespace,
	}, secret); err != nil {
		return nil, k8sErrorToGRPCError(err)
	}
	passphrase, ok := secret.Data[ref.Key]
	if !ok || len(passphrase) == 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "source pgbackrest cipher secret %s does not contain key %s", secret.Name, ref.Key)
	}
	return bytes.Clone(passphrase), nil
}

func generatePgBackRestCipherPassphrase() ([]byte, error) {
	raw := make([]byte, pgBackRestCipherBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	encoded := make([]byte, base64.RawURLEncoding.EncodedLen(len(raw)))
	base64.RawURLEncoding.Encode(encoded, raw)
	return encoded, nil
}

func checkPgBackRestSecret(
	secret *corev1.Secret,
	requireTarget bool,
	restorePassphrase []byte,
) (bool, error) {
	if secret.Immutable == nil || !*secret.Immutable {
		return false, status.Errorf(codes.FailedPrecondition, "pgbackrest cipher secret %s is not immutable", secret.Name)
	}
	target, hasTarget := secret.Data[PgBackRestCipherPassphraseKey]
	if hasTarget && len(target) == 0 {
		return false, status.Errorf(codes.FailedPrecondition, "pgbackrest cipher secret %s contains an empty target cipher", secret.Name)
	}
	if requireTarget && !hasTarget {
		return false, status.Errorf(codes.FailedPrecondition, "pgbackrest cipher secret %s does not contain a target cipher", secret.Name)
	}

	restored, hasRestore := secret.Data[PgBackRestRestorePassphraseKey]
	if restorePassphrase == nil {
		return hasTarget, nil
	}
	if !hasRestore || !bytes.Equal(restored, restorePassphrase) {
		return false, status.Errorf(codes.FailedPrecondition, "pgbackrest cipher secret %s does not match the restore source", secret.Name)
	}
	return hasTarget, nil
}

func setPgBackRestCipherReferences(branch *branchv1alpha1.Branch, hasTargetPassphrase, hasRestorePassphrase bool) {
	if hasTargetPassphrase {
		branch.Spec.BackupSpec.PgBackRest.CipherPassphraseSecretRef = pgBackRestSecretKeySelector(
			branch.Name, PgBackRestCipherPassphraseKey,
		)
	}
	if hasRestorePassphrase {
		branch.Spec.Restore.PgBackRestCipherPassphraseSecretRef = pgBackRestSecretKeySelector(
			branch.Name, PgBackRestRestorePassphraseKey,
		)
	}
}

func pgBackRestSecretKeySelector(branchID, key string) *corev1.SecretKeySelector {
	return &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: branchID + PgBackRestSecretSuffix},
		Key:                  key,
	}
}
