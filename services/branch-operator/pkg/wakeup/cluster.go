package wakeup

import (
	"context"
	"fmt"
	"time"

	"xata/services/branch-operator/api/v1alpha1"
	"xata/services/branch-operator/pkg/shared"

	"github.com/go-logr/logr"
	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	apiv1ac "github.com/xataio/xata-cnpg/pkg/client/applyconfiguration/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	rolePasswordSyncInterval = 100 * time.Millisecond
	rolePasswordSyncTimeout  = 30 * time.Second
)

// setUserPasswordSecret updates the Cluster resource to set the user password
// secret for the 'xata' role to the secret associated with the given Branch
func (r *WakeupReconciler) setUserPasswordSecret(ctx context.Context, branch *v1alpha1.Branch, cluster *apiv1.Cluster) error {
	ac := apiv1ac.Cluster(cluster.Name, cluster.Namespace).
		WithSpec(apiv1ac.ClusterSpec().
			WithManaged(apiv1ac.ManagedConfiguration().
				WithRoles(shared.XataRoleConfiguration(branch.Name))))

	return r.Apply(ctx, ac, client.FieldOwner(ReconcilerName), client.ForceOwnership)
}

// waitForRolePasswordSync polls the Cluster's status until it reports that the
// 'xata' role is using a password derived from the current version of the
// Branch's user password secret
func (r *WakeupReconciler) waitForRolePasswordSync(
	ctx context.Context,
	log logr.Logger,
	branchName string,
	cluster *apiv1.Cluster,
) error {
	// Get the branch password secret for the 'xata' user
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      branchName + "-app",
		Namespace: cluster.Namespace,
	}, secret)
	if err != nil {
		return fmt.Errorf("get branch secret %q: %w", branchName+"-app", err)
	}

	// Poll the Cluster's status until it reports that the 'xata' user role is
	// using the password from the current version of the Branch secret
	return wait.PollUntilContextTimeout(ctx, rolePasswordSyncInterval, rolePasswordSyncTimeout, true,
		func(ctx context.Context) (bool, error) {
			if err := r.Get(ctx, client.ObjectKeyFromObject(cluster), cluster); err != nil {
				return false, err
			}
			if clusterUsesCredsFromSecretVersion(cluster, shared.XataRoleName, secret.ResourceVersion) {
				return true, nil
			}
			log.Info("waiting for cluster to sync user password secret", "cluster", cluster.Name, "secret", secret.Name)
			return false, nil
		})
}

// clusterUsesCredsFromSecretVersion checks if the given Cluster's status
// reports that the specified user role is using a password derived from the
// given secret version
func clusterUsesCredsFromSecretVersion(cluster *apiv1.Cluster, username, secretVersion string) bool {
	st, ok := cluster.Status.ManagedRolesStatus.PasswordStatus[username]
	return ok && st.SecretResourceVersion == secretVersion
}
