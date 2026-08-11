package reconciler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"xata/services/branch-operator/api/v1alpha1"
)

func TestReconcilePgBackRestSecretsAdoptsExistingSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	branch := &v1alpha1.Branch{
		ObjectMeta: metav1.ObjectMeta{Name: "branch", UID: types.UID("branch-uid")},
		Spec: v1alpha1.BranchSpec{
			BackupSpec: &v1alpha1.BackupSpec{
				PgBackRest: &v1alpha1.PgBackRestSpec{
					CipherPassphraseSecretRef: secretKeySelector("branch-pgbackrest", "target"),
				},
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "branch-pgbackrest", Namespace: "xata-clusters"},
		Immutable:  new(true),
		Data:       map[string][]byte{"target": []byte("unchanged")},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(branch, secret).Build()
	r := &BranchReconciler{Client: client, Scheme: scheme, ClustersNamespace: "xata-clusters"}

	_, err := r.reconcilePgBackRestSecrets(context.Background(), branch)
	require.NoError(t, err)

	got := &corev1.Secret{}
	require.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, got))
	require.Equal(t, []byte("unchanged"), got.Data["target"])
	require.Len(t, got.OwnerReferences, 1)
	require.Equal(t, branch.Name, got.OwnerReferences[0].Name)
}

func secretKeySelector(name, key string) *corev1.SecretKeySelector {
	return &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: name},
		Key:                  key,
	}
}
