package clusters

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clustersv1 "xata/gen/proto/clusters/v1"
	"xata/internal/passwords"
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
