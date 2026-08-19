package reconciler

import (
	"context"

	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"xata/services/branch-operator/api/v1alpha1"
)

// updateBackupStatus copies the backup status fields from the Branch's
// Cluster onto the Branch status. Values are only overwritten with non-empty
// data, so the Branch keeps the last known values across hibernation and
// cluster replacement (a fresh pool cluster starts with an empty status).
func (r *BranchReconciler) updateBackupStatus(ctx context.Context, branch *v1alpha1.Branch) error {
	// Without a cluster there is nothing to copy; keep the recorded values
	if !branch.HasClusterName() {
		return nil
	}

	cluster := &apiv1.Cluster{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      branch.ClusterName(),
		Namespace: r.ClustersNamespace,
	}, cluster)
	if err != nil {
		// The cluster may not exist yet; keep the recorded values
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// The fields are marked deprecated upstream because backup plugins do not
	// set them; our in-core pgbackrest does set them.
	if cluster.Status.FirstRecoverabilityPoint != "" { //nolint:staticcheck
		branch.Status.FirstRecoverabilityPoint = cluster.Status.FirstRecoverabilityPoint //nolint:staticcheck
	}
	if cluster.Status.LastSuccessfulBackup != "" { //nolint:staticcheck
		branch.Status.LastSuccessfulBackup = cluster.Status.LastSuccessfulBackup //nolint:staticcheck
	}
	if cluster.Status.LastFailedBackup != "" { //nolint:staticcheck
		branch.Status.LastFailedBackup = cluster.Status.LastFailedBackup //nolint:staticcheck
	}
	if cluster.Status.LastRecoverabilityPoint != "" {
		branch.Status.LastRecoverabilityPoint = cluster.Status.LastRecoverabilityPoint
	}

	return nil
}
