package reconciler

import (
	"context"

	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"xata/services/branch-operator/api/v1alpha1"
	"xata/services/branch-operator/pkg/reconciler/resources"
)

const PoolerSuffix = "-pooler"

// reconcilePooler ensures that the correct Pooler exists for the given Branch
// when a pooler is configured. When Pooler is nil, it ensures no Pooler exists.
func (r *BranchReconciler) reconcilePooler(
	ctx context.Context,
	branch *v1alpha1.Branch,
) (controllerutil.OperationResult, error) {
	pooler := &apiv1.Pooler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      branch.Name + PoolerSuffix,
			Namespace: r.ClustersNamespace,
		},
	}

	// If pooler is not configured, ensure Pooler doesn't exist
	if !branch.Spec.Pooler.IsEnabled() {
		err := r.Get(ctx, types.NamespacedName{
			Name:      branch.Name + PoolerSuffix,
			Namespace: r.ClustersNamespace,
		}, pooler)
		if err != nil {
			return controllerutil.OperationResultNone, client.IgnoreNotFound(err)
		}

		if err := r.Delete(ctx, pooler); err != nil {
			return controllerutil.OperationResultNone, err
		}
		return controllerutil.OperationResultUpdated, nil
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, pooler, func() error {
		if err := controllerutil.SetControllerReference(branch, pooler, r.Scheme); err != nil {
			return err
		}

		ensureLabels(&pooler.ObjectMeta, branch.Spec.InheritedMetadata)

		pooler.Spec = resources.PoolerSpec(
			branch.ClusterName(),
			branch.Spec.Pooler.Instances,
			!branch.HasClusterName() || branch.Spec.ClusterSpec.Hibernation.IsEnabled(),
			apiv1.PgBouncerPoolMode(branch.Spec.Pooler.Mode),
			branch.Spec.Pooler.MaxClientConn,
			branch.Spec.InheritedMetadata.GetLabels(),
			r.ImagePullSecrets,
		)

		return nil
	})

	return result, err
}
