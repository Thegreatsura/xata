package wakeup

import (
	"context"
	"fmt"
	"slices"

	poolv1alpha1 "xata/proto/clusterpool-operator/api/v1alpha1"
	"xata/services/branch-operator/api/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// takeClusterFromPool attempts to remove a Cluster from the specified
// ClusterPool. It does this by listing all Clusters owned by the pool and
// removing the pool's controller owner reference from one of them.
func (r *WakeupReconciler) takeClusterFromPool(ctx context.Context, namespace, poolName string) (*apiv1.Cluster, error) {
	// Get the ClusterPool resource by name.
	pool := &poolv1alpha1.ClusterPool{}
	err := r.Get(ctx, client.ObjectKey{Name: poolName, Namespace: namespace}, pool)
	if err != nil && apierrors.IsNotFound(err) {
		err = &ConditionError{
			ConditionReason: v1alpha1.PoolNotFoundReason,
			Err:             err,
			Terminal:        true,
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	// List clusters in the pool that are available for claiming. The
	// PoolClusterOwnerKey index allows efficient querying of clusters by their
	// owning pool.
	var clusterList apiv1.ClusterList
	err = r.List(ctx, &clusterList,
		client.InNamespace(namespace),
		client.MatchingFields{PoolClusterOwnerKey: poolName},
	)
	if err != nil {
		return nil, err
	}

	// Filter to consider only Clusters that have ready instances
	clusterList.Items = slices.DeleteFunc(clusterList.Items,
		func(c apiv1.Cluster) bool {
			return c.Status.ReadyInstances == 0
		})

	// Ensure that there is at least one available Cluster to claim. If not,
	// return a ConditionError indicating that the pool is exhausted.
	if len(clusterList.Items) == 0 {
		return nil, &ConditionError{
			ConditionReason: v1alpha1.PoolExhaustedReason,
			Err:             fmt.Errorf("no clusters in pool %q", poolName),
		}
	}

	// Claim the cluster by removing the pool's controller owner reference.
	// This uses optimistic concurrency via ResourceVersion — if another
	// WakeupRequest claimed this cluster first, the update returns a conflict
	// and the reconciler requeues.
	cluster := &clusterList.Items[0]
	if err = controllerutil.RemoveControllerReference(pool, cluster, r.Scheme); err != nil {
		return nil, fmt.Errorf("remove controller reference: %w", err)
	}
	if err = r.Update(ctx, cluster); err != nil {
		return nil, err
	}

	return cluster, nil
}
